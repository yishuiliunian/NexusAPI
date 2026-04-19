// Package task 提供异步任务服务：提交、查询、轮询。
//
// DIP：通过 TaskLookup 函数注入 provider，不直接 import infra/provider。
//
// 计费语义：
//   - Submit 时按 ModelPrice.TaskPrice 调 Biller.Charge 扣费，Ledger.Type = TaskCharge。
//   - 上游 Submit 失败或任务执行失败：Biller.TopUp 退回，Ledger.Type = TaskRefund。
//   - Task.Cost 保留历史扣费金额（即使退款也不清零）；Task.Refunded 作为幂等标记。
package task

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	domaintask "github.com/yishuiliunian/nexusapi/backend/internal/domain/task"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// TaskLookup 通过 provider 名字返回 TaskAdaptor。不存在返回 nil。
type TaskLookup func(name string) relay.TaskAdaptor

// Biller 任务计费所需的 billing 接口子集（避免循环依赖）。
type Biller interface {
	// Charge 扣款（amount 正数，内部转负）。余额不足返回 ErrInsufficientQuota。
	Charge(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error
	// TopUp 充值/退款（amount 正数）。
	TopUp(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error
}

// PriceLookup 读取任务计费价格。
type PriceLookup interface {
	Get(ctx context.Context, model string, cap billing.Capability) (*billing.ModelPrice, error)
}

// Service 任务编排服务。
type Service struct {
	Tasks    domaintask.Repository
	Channels domainchannel.Repository
	Prices   PriceLookup
	Biller   Biller
	lookup   TaskLookup
}

// NewService 构造。
func NewService(
	tasks domaintask.Repository,
	channels domainchannel.Repository,
	prices PriceLookup,
	biller Biller,
	lookup TaskLookup,
) *Service {
	return &Service{
		Tasks:    tasks,
		Channels: channels,
		Prices:   prices,
		Biller:   biller,
		lookup:   lookup,
	}
}

// SubmitInput 创建一个任务。
type SubmitInput struct {
	UserID   uint64
	ApiKeyID uint64
	Provider string
	Action   string
	Model    string
	Input    any
}

// Submit 创建任务并立即向上游 Submit。
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*domaintask.Task, error) {
	ch, ad, err := s.resolveChannel(ctx, in.Provider)
	if err != nil {
		return nil, err
	}

	cost := s.priceTask(ctx, in.Model, ch.PriceMultiplier)
	taskID := uuid.NewString()

	if err := s.chargeIfNeeded(ctx, in.UserID, cost, taskID); err != nil {
		return nil, err
	}

	externalID, err := ad.Submit(ctx, toUpstream(ch), in.Action, in.Input)
	if err != nil {
		s.refundOnFail(ctx, in.UserID, cost, taskID, "任务提交失败退款")
		return nil, derrors.Wrap(derrors.CodeUpstream, "submit task", err)
	}

	t := s.buildTask(in, ch.ID, taskID, externalID, cost)
	if err := s.Tasks.Create(ctx, t); err != nil {
		s.refundOnFail(ctx, in.UserID, cost, taskID, "任务记录失败退款")
		return nil, err
	}
	return t, nil
}

// resolveChannel 选渠道 + 取 TaskAdaptor。
func (s *Service) resolveChannel(ctx context.Context, provider string) (*domainchannel.Channel, relay.TaskAdaptor, error) {
	channels, err := s.Channels.ListActive(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, c := range channels {
		if c.Provider == provider {
			ad := s.lookup(provider)
			if ad == nil {
				return nil, nil, derrors.New(derrors.CodeInvalidArgument, "provider 不支持任务: "+provider)
			}
			return c, ad, nil
		}
	}
	return nil, nil, derrors.New(derrors.CodeNotFound, "无可用 "+provider+" 渠道")
}

// priceTask 计算单次任务扣费。无价格配置返回 0。
func (s *Service) priceTask(ctx context.Context, model string, channelMul float64) int64 {
	if model == "" || s.Prices == nil {
		return 0
	}
	p, err := s.Prices.Get(ctx, model, billing.CapTask)
	if err != nil || p == nil || !p.Enabled || p.TaskPrice <= 0 {
		return 0
	}
	mul := channelMul
	if mul <= 0 {
		mul = 1
	}
	return int64(float64(p.TaskPrice) * mul)
}

// chargeIfNeeded cost>0 时扣费。
func (s *Service) chargeIfNeeded(ctx context.Context, userID uint64, cost int64, taskID string) error {
	if cost <= 0 || s.Biller == nil {
		return nil
	}
	return s.Biller.Charge(ctx, userID, cost, billing.LedgerTaskCharge, taskID, "任务扣费")
}

// refundOnFail 任务失败场景退费（不检查 Refunded 标记，因为 Task 还没入库）。
func (s *Service) refundOnFail(ctx context.Context, userID uint64, cost int64, taskID, note string) {
	if cost <= 0 || s.Biller == nil {
		return
	}
	_ = s.Biller.TopUp(ctx, userID, cost, billing.LedgerTaskRefund, taskID, note)
}

// buildTask 组装待入库的 Task 实体。
func (s *Service) buildTask(in SubmitInput, channelID uint64, taskID, externalID string, cost int64) *domaintask.Task {
	inputJSON, _ := json.Marshal(in.Input)
	return &domaintask.Task{
		ID:         taskID,
		UserID:     in.UserID,
		ApiKeyID:   in.ApiKeyID,
		ChannelID:  channelID,
		Provider:   in.Provider,
		Action:     in.Action,
		Model:      in.Model,
		Input:      inputJSON,
		Status:     domaintask.StatusPending,
		ExternalID: externalID,
		Cost:       cost,
		CreatedAt:  time.Now(),
	}
}

// Get 查询任务。
func (s *Service) Get(ctx context.Context, id string) (*domaintask.Task, error) {
	return s.Tasks.GetByID(ctx, id)
}

// ListByUser 列用户任务。
func (s *Service) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*domaintask.Task, int64, error) {
	return s.Tasks.ListByUser(ctx, userID, offset, limit)
}

// Poll 轮询一个任务。
func (s *Service) Poll(ctx context.Context, t *domaintask.Task) error {
	ad := s.lookup(t.Provider)
	if ad == nil {
		t.Status = domaintask.StatusFailed
		t.Error = "provider 不支持任务"
		s.refundPersistedTask(ctx, t)
		return s.Tasks.Update(ctx, t)
	}
	ch, err := s.Channels.GetByID(ctx, t.ChannelID)
	if err != nil {
		return err
	}
	res, err := ad.Query(ctx, toUpstream(ch), t.ExternalID)
	if err != nil {
		return err
	}
	t.Progress = res.Progress
	if res.Result != nil {
		rjson, _ := json.Marshal(res.Result)
		t.Result = rjson
	}
	if t.StartedAt == nil && res.Status == relay.TaskRunning {
		now := time.Now()
		t.StartedAt = &now
	}
	switch res.Status {
	case relay.TaskRunning:
		t.Status = domaintask.StatusRunning
	case relay.TaskSuccess:
		t.Status = domaintask.StatusSuccess
		t.Progress = 100
		now := time.Now()
		t.FinishedAt = &now
	case relay.TaskFailed:
		t.Status = domaintask.StatusFailed
		t.Error = res.Error
		now := time.Now()
		t.FinishedAt = &now
		s.refundPersistedTask(ctx, t)
	}
	return s.Tasks.Update(ctx, t)
}

// refundPersistedTask 任务失败退款（针对已入库的 Task）。
// 幂等：通过 Task.Refunded 判断，保留 Cost 历史值。
func (s *Service) refundPersistedTask(ctx context.Context, t *domaintask.Task) {
	if t.Cost <= 0 || t.Refunded || s.Biller == nil {
		return
	}
	if err := s.Biller.TopUp(ctx, t.UserID, t.Cost, billing.LedgerTaskRefund, t.ID, "任务失败退款"); err == nil {
		t.Refunded = true
	}
}

func toUpstream(ch *domainchannel.Channel) relay.Upstream {
	return relay.Upstream{
		ID:          ch.ID,
		Provider:    ch.Provider,
		BaseURL:     ch.BaseURL,
		Credentials: ch.Credentials,
	}
}
