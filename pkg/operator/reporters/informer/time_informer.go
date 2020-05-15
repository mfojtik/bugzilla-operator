package informer

import (
	"context"
	"sync"

	"github.com/robfig/cron/v3"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type TimeInformer struct {
	c         *cron.Cron
	handlers  []cache.ResourceEventHandler
	schedules []string
	started   bool
	sync.Mutex
}

func NewTimeInformer() *TimeInformer {
	return &TimeInformer{
		c: cron.New(),
	}
}

func (t *TimeInformer) Schedule(schedule string) {
	t.schedules = append(t.schedules, schedule)
}

func (t *TimeInformer) Start(ctx context.Context) {

	for _, schedule := range t.schedules {
		id, err := t.c.AddFunc(schedule, func() {
			for i := range t.handlers {
				klog.Infof("Triggering run via cron schedule %q", schedule)
				t.handlers[i].OnAdd("cron")
			}
		})
		if err != nil {
			panic(err)
		}
		klog.Infof("Scheduled controller run #%d: %q", id, schedule)
	}

	go t.c.Start()
	t.Lock()
	t.started = true
	t.Unlock()
	<-ctx.Done()
	t.c.Stop()
}

func (t *TimeInformer) AddEventHandler(handler cache.ResourceEventHandler) {
	t.handlers = append(t.handlers, handler)
}

func (t *TimeInformer) HasSynced() bool {
	t.Lock()
	defer t.Unlock()
	return t.started
}
