package informer

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/tools/cache"
)

func TestNewTimeInformer(t *testing.T) {
	i := NewTimeInformer()
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
