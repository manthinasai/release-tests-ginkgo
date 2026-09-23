# Dockerfile — Interop / downstream test image
# Layers fresh test source onto the pre-built CI tooling image (built by Dockerfile.CI).
# Used by interop and partner teams to run release tests ginkgo without rebuilding all tooling.

FROM quay.io/openshift-pipeline/release-tests-ginkgo:latest

RUN mkdir -p /tmp/release-tests-ginkgo
WORKDIR /tmp/release-tests-ginkgo
COPY . .

RUN chgrp -R 0 /tmp && \
    chmod -R g=u /tmp

CMD ["/bin/bash"]
