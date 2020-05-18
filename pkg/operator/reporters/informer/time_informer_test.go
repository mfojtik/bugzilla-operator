package informer

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/client-go/tools/cache"
)

func TestTimeInformerInFactory(t *testing.T) {
	i := NewTimeInformer("test")
	i.Schedule("@every 1s")

	queueKey := ""
	syncChan := make(chan struct{})
	c := factory.New().WithInformers(i).WithSync(func(ctx context.Context, controllerContext factory.SyncContext) error {
		queueKey = controllerContext.QueueKey()
		syncChan <- struct{}{}
		return nil
	}).ToController("Test", events.NewInMemoryRecorder("test"))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go i.Start(ctx)
	go c.Run(ctx, 1)

	select {
	case <-syncChan:
	case <-time.After(10 * time.Second):
		t.Errorf("timeout")
	}
	if queueKey != factory.DefaultQueueKey {
		t.Errorf("expected queue key %q, got %q", factory.DefaultQueueKey, queueKey)
	}

}

func TestNewTimeInformer(t *testing.T) {
	i := NewTimeInformer("test")
	triggerChan := make(chan struct{})
	i.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			triggerChan <- struct{}{}
		},
	})

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	i.Schedule("@every 1s")

	go i.Start(ctx)

	if !cache.WaitForNamedCacheSync("Test", ctx.Done(), i.HasSynced) {
		t.Fatal("hasSynced() broken")
	}

	triggerCount := 0
	for {
		select {
		case <-triggerChan:
			triggerCount++
			t.Logf("trigger recv")
			if triggerCount > 1 {
				return
			}
		case <-time.After(10 * time.Second):
			t.Errorf("timeout")
			return
		}
	}
}
