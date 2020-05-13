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

clean:
	$(RM) ./bugzilla-operator
.PHONY: clean