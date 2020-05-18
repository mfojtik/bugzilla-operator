package informer

import (
	"context"
	"sync"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type CronObject struct {
}

func (c *CronObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (c *CronObject) DeepCopyObject() runtime.Object {
	return c
}

type TimeInformer struct {
	c         *cron.Cron
	name      string
	handlers  []cache.ResourceEventHandler
	schedules []string
	started   bool
	sync.Mutex
}

func NewTimeInformer(name string) *TimeInformer {
	return &TimeInformer{
		c:    cron.New(),
		name: name,
	}
}

func (t *TimeInformer) Schedule(schedule string) {
	t.schedules = append(t.schedules, schedule)
}

func (t *TimeInformer) Start(ctx context.Context) {
	for _, schedule := range t.schedules {
		id, err := t.c.AddFunc(schedule, func() {
			for i := range t.handlers {
				klog.Infof("Triggering run using %q via schedule %q", t.name, schedule)
				t.handlers[i].OnAdd(&CronObject{})
			}
		})
		if err != nil {
			panic(err)
		}
		klog.Infof("Scheduled controller %q run #%d: %q", t.name, id, schedule)
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
