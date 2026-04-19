package queue

// 任务类型常量。新增任务时在此注册，worker 端 handler 通过相同常量订阅。
const (
	// TaskPing 健康探针。真实异步任务由 Worker 的 TaskPoller 定期扫库完成，
	// 暂不走 asynq 投递队列。
	TaskPing = "system:ping"
)
