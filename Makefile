all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps-gomod.mk \
	targets/openshift/images.mk \
)

IMAGE_REGISTRY?=quay.io

$(call build-image,bugzilla-operator,$(IMAGE_REGISTRY)/mfojtik/bugzilla-operator:dev,./Dockerfile,.)

install:
	kubectl apply -f ./manifests
	# You must provide Bugzilla credentials via: kubectl edit configmap/operator-config"
.PHONY: install

uninstall:
	kubectl delete namespace/bugzilla-operator
.PHONY: uninstall

push:
	docker push quay.io/mfojtik/bugzilla-operator:dev
.PHONY: push

clean:
	$(RM) ./bugzilla-operator
.PHONY: clean