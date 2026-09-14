package collector

import (
	"context"
	"log"
	"sync"
	"time"
)

type StorageWriter interface {
	InsertUsageBatch(events []*TokenUsageEvent) error
}

type AgentTaskWriter interface {
	InsertAgentTaskBatch(tasks []*AgentTaskEvent) error
}

type AsyncBuffer struct {
	eventChan chan *TokenUsageEvent
	writer    StorageWriter
	batchSize int
	flushTick time.Duration
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewAsyncBuffer(writer StorageWriter, capacity, batchSize int, flushTick time.Duration) *AsyncBuffer {
	if capacity <= 0 {
		capacity = 1000
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushTick <= 0 {
		flushTick = 1 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncBuffer{
		eventChan: make(chan *TokenUsageEvent, capacity),
		writer:    writer,
		batchSize: batchSize,
		flushTick: flushTick,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (b *AsyncBuffer) Start() {
	b.wg.Add(1)
	go b.worker()
}

func (b *AsyncBuffer) Push(event *TokenUsageEvent) bool {
	select {
	case b.eventChan <- event:
		return true
	default:
		log.Printf("[WARN] In-memory buffer đầy (%d), bỏ qua hoặc chuyển sang spool", cap(b.eventChan))
		return false
	}
}

// FlushDirect ghi trực tiếp một batch vào storage không qua queue
func (b *AsyncBuffer) FlushDirect(events []*TokenUsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	return b.writer.InsertUsageBatch(events)
}

// FlushAgentTasksDirect ghi trực tiếp một batch agent tasks vào storage nếu writer hỗ trợ
func (b *AsyncBuffer) FlushAgentTasksDirect(tasks []*AgentTaskEvent) error {
	if len(tasks) == 0 || b.writer == nil {
		return nil
	}
	if atw, ok := b.writer.(AgentTaskWriter); ok {
		return atw.InsertAgentTaskBatch(tasks)
	}
	return nil
}

func (b *AsyncBuffer) worker() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.flushTick)
	defer ticker.Stop()

	batch := make([]*TokenUsageEvent, 0, b.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := b.writer.InsertUsageBatch(batch); err != nil {
			log.Printf("[ERROR] Lỗi flush batch %d sự kiện vào database: %v", len(batch), err)
		}
		batch = make([]*TokenUsageEvent, 0, b.batchSize)
	}

	for {
		select {
		case <-b.ctx.Done():
			// Rút hết các event còn lại trong channel trước khi dừng
			for {
				select {
				case ev := <-b.eventChan:
					batch = append(batch, ev)
					if len(batch) >= b.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}

		case ev := <-b.eventChan:
			batch = append(batch, ev)
			if len(batch) >= b.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

func (b *AsyncBuffer) Stop() {
	b.cancel()
	b.wg.Wait()
}
