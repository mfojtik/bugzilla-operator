FROM registry.ci.openshift.org/openshift/release:golang-1.13 AS builder
WORKDIR /go/src/github.com/mfojtik/bugzilla-operator
COPY . .
ENV GO_PACKAGE github.com/mfojtik/bugzilla-operator
RUN make build --warn-undefined-variables

FROM registry.ci.openshift.org/ocp/4.6:base
COPY --from=builder /go/src/github.com/mfojtik/bugzilla-operator/bugzilla-operator /usr/bin/

