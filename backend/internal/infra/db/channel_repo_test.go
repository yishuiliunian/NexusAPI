package db

import (
	"context"
	"testing"
	"time"

	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
)

func mkChannel(t *testing.T, r *ChannelRepo, overrides ...func(c *channel.Channel)) *channel.Channel {
	t.Helper()
	c := &channel.Channel{
		Name:            "test",
		Provider:        "openai",
		BaseURL:         "https://api.openai.com/v1",
		Credentials:     "sk-plain",
		Models:          []string{"gpt-4o", "gpt-4o-mini"},
		Weight:          100,
		PriceMultiplier: 1.0,
		Status:          channel.StatusActive,
	}
	for _, o := range overrides {
		o(c)
	}
	if err := r.Create(context.Background(), c); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return c
}

func TestChannelRepo_CreateAndEncryption(t *testing.T) {
	// 用 AES cipher 验证凭据加密
	key := make([]byte, 32)
	copy(key, []byte("0123456789abcdef0123456789abcdef"))
	cipher, err := cryptoutil.New(key)
	if err != nil {
		t.Fatal(err)
	}
	d := newTestDB(t)
	r := NewChannelRepo(d, cipher)
	c := mkChannel(t, r, func(c *channel.Channel) { c.Credentials = "sk-secret-original" })

	// 从 repo 读回应该是明文
	got, err := r.GetByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Credentials != "sk-secret-original" {
		t.Errorf("解密失败: %q", got.Credentials)
	}
	// 直接查 DB 应该是密文
	var row ChannelRow
	_ = d.First(&row, c.ID).Error
	if row.Credentials == "sk-secret-original" {
		t.Errorf("凭据未加密落库: %q", row.Credentials)
	}
}

func TestChannelRepo_ListActive(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	mkChannel(t, r, func(c *channel.Channel) { c.Name = "active1"; c.Status = channel.StatusActive })
	mkChannel(t, r, func(c *channel.Channel) { c.Name = "disabled"; c.Status = channel.StatusDisabled })
	mkChannel(t, r, func(c *channel.Channel) { c.Name = "testing"; c.Status = channel.StatusTesting })

	out, err := r.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "active1" {
		t.Errorf("ListActive 应只返回 active，got %+v", out)
	}
}

func TestChannelRepo_GroupsRoundTrip(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	c := mkChannel(t, r, func(c *channel.Channel) { c.GroupIDs = []uint64{10, 20, 30} })
	got, err := r.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GroupIDs) != 3 {
		t.Errorf("GroupIDs 未水合: %+v", got.GroupIDs)
	}

	// Update 也要正确
	c.GroupIDs = []uint64{99}
	if err := r.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(ctx, c.ID)
	if len(got.GroupIDs) != 1 || got.GroupIDs[0] != 99 {
		t.Errorf("Update 后 GroupIDs 错: %+v", got.GroupIDs)
	}
}

func TestChannelRepo_UpdateHealth(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()
	c := mkChannel(t, r)

	now := time.Now().Truncate(time.Second)
	if err := r.UpdateHealth(ctx, c.ID, 150, now); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, c.ID)
	if got.LatencyMs != 150 {
		t.Errorf("latency 未更新: %d", got.LatencyMs)
	}
	if got.TestedAt == nil || !got.TestedAt.Equal(now) {
		t.Errorf("TestedAt 未更新: %v", got.TestedAt)
	}
}

func TestChannelRepo_DeleteAndNotFound(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()
	c := mkChannel(t, r)
	if err := r.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByID(ctx, c.ID); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("删除后应 NotFound, got %v", err)
	}
}

// TestChannelRepo_UsersAndApiKeysRoundTrip 验证用户级 / ApiKey 级白名单的全量写入与回读。
func TestChannelRepo_UsersAndApiKeysRoundTrip(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	c := mkChannel(t, r, func(c *channel.Channel) {
		c.UserIDs = []uint64{100, 200, 300}
		c.ApiKeyIDs = []uint64{11, 22}
	})
	got, err := r.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.UserIDs) != 3 {
		t.Errorf("UserIDs 未水合: %+v", got.UserIDs)
	}
	if len(got.ApiKeyIDs) != 2 {
		t.Errorf("ApiKeyIDs 未水合: %+v", got.ApiKeyIDs)
	}

	// Update 走全量替换语义
	c.UserIDs = []uint64{500}
	c.ApiKeyIDs = nil // 清空恢复"不限制"
	if err := r.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(ctx, c.ID)
	if len(got.UserIDs) != 1 || got.UserIDs[0] != 500 {
		t.Errorf("Update 后 UserIDs 错: %+v", got.UserIDs)
	}
	if len(got.ApiKeyIDs) != 0 {
		t.Errorf("Update 后 ApiKeyIDs 应清空: %+v", got.ApiKeyIDs)
	}
}

// TestChannelRepo_DeleteCascadeGrants 验证渠道删除时关联白名单同事务清理。
func TestChannelRepo_DeleteCascadeGrants(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	c := mkChannel(t, r, func(c *channel.Channel) {
		c.GroupIDs = []uint64{1}
		c.UserIDs = []uint64{100}
		c.ApiKeyIDs = []uint64{77}
	})
	if err := r.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}

	// 直接查关联表，应该都为空
	var gCount, uCount, kCount int64
	r.db.Model(&ChannelGroupRow{}).Where("channel_id = ?", c.ID).Count(&gCount)
	r.db.Model(&ChannelUserRow{}).Where("channel_id = ?", c.ID).Count(&uCount)
	r.db.Model(&ChannelApiKeyRow{}).Where("channel_id = ?", c.ID).Count(&kCount)
	if gCount+uCount+kCount != 0 {
		t.Errorf("级联清理失败: groups=%d users=%d apikeys=%d", gCount, uCount, kCount)
	}
}

// TestChannelRepo_HydrateBatchLoadsAllGrants 验证 List/ListActive 批量加载三层白名单。
func TestChannelRepo_HydrateBatchLoadsAllGrants(t *testing.T) {
	r := NewChannelRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	c1 := mkChannel(t, r, func(c *channel.Channel) {
		c.Name = "ch1"
		c.GroupIDs = []uint64{1, 2}
		c.UserIDs = []uint64{100}
	})
	c2 := mkChannel(t, r, func(c *channel.Channel) {
		c.Name = "ch2"
		c.UserIDs = []uint64{200, 300}
		c.ApiKeyIDs = []uint64{99}
	})

	out, err := r.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[uint64]*channel.Channel{}
	for _, ch := range out {
		byID[ch.ID] = ch
	}
	if len(byID[c1.ID].GroupIDs) != 2 || len(byID[c1.ID].UserIDs) != 1 {
		t.Errorf("c1 hydrate 错: groups=%v users=%v", byID[c1.ID].GroupIDs, byID[c1.ID].UserIDs)
	}
	if len(byID[c2.ID].UserIDs) != 2 || len(byID[c2.ID].ApiKeyIDs) != 1 {
		t.Errorf("c2 hydrate 错: users=%v apikeys=%v", byID[c2.ID].UserIDs, byID[c2.ID].ApiKeyIDs)
	}
}
