# The toolbox: each task operates in this image, on a laptop and in CI. Go builds the
# provider for all release platforms from this one image, because the build uses no cgo.
FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
# The mount does not always contain .git, and no code reads the VCS stamp.
ENV GOFLAGS=-buildvcs=false CGO_ENABLED=0
# GoReleaser needs git for a release, and gpg to sign the checksums.
RUN apk add --no-cache git gnupg

# The acceptance tests run each CLI, GoReleaser makes the release archives, and
# golangci-lint examines the code. Each
# download agrees with the SHA256SUMS file of its release. The hash of that file is in
# this Dockerfile. Thus, one hash for each tool covers each architecture. To change a
# version, change the version and the hash of its SHA256SUMS file together.
ARG TERRAFORM=1.16.5
ARG TERRAFORM_SUMS=909485e738f17e345047a74c06644a5a5b5f1d6e5bf2a4d10e0d416342db8e77
ARG TOFU=1.13.1
ARG TOFU_SUMS=f4a03dd8613320c665848cb6e1962ee112d777c9c6a8a725586dcc341b1c7252
ARG GORELEASER=2.18.3
ARG GORELEASER_SUMS=a40282e5419a5e7176c78afae243777ae0da7376e165d8cf59805910d2c84e1e
ARG GOLANGCI_LINT=2.14.0
ARG GOLANGCI_LINT_SUMS=7b4eeb888873ac45ec58a640c3243c43a3540e097f6c4410efd2bdb306340023

RUN set -eu; \
    case "$(apk --print-arch)" in \
      x86_64)  arch=amd64; grarch=x86_64 ;; \
      aarch64) arch=arm64; grarch=arm64 ;; \
      *) echo "unsupported architecture: $(apk --print-arch)" >&2; exit 1 ;; \
    esac; \
    cd /tmp; \
    get() { url=$1; sums=$2; hash=$3; file=$4; \
      wget -q -O "$sums" "${url%/*}/$sums"; \
      echo "$hash  $sums" | sha256sum -c -; \
      wget -q -O "$file" "$url"; \
      grep " $file\$" "$sums" | sha256sum -c -; }; \
    get "https://releases.hashicorp.com/terraform/${TERRAFORM}/terraform_${TERRAFORM}_linux_${arch}.zip" \
      "terraform_${TERRAFORM}_SHA256SUMS" "$TERRAFORM_SUMS" "terraform_${TERRAFORM}_linux_${arch}.zip"; \
    unzip -q -o "terraform_${TERRAFORM}_linux_${arch}.zip" terraform -d /usr/local/bin; \
    get "https://github.com/opentofu/opentofu/releases/download/v${TOFU}/tofu_${TOFU}_linux_${arch}.tar.gz" \
      "tofu_${TOFU}_SHA256SUMS" "$TOFU_SUMS" "tofu_${TOFU}_linux_${arch}.tar.gz"; \
    tar -xzf "tofu_${TOFU}_linux_${arch}.tar.gz" -C /usr/local/bin tofu; \
    get "https://github.com/goreleaser/goreleaser/releases/download/v${GORELEASER}/goreleaser_Linux_${grarch}.tar.gz" \
      checksums.txt "$GORELEASER_SUMS" "goreleaser_Linux_${grarch}.tar.gz"; \
    tar -xzf "goreleaser_Linux_${grarch}.tar.gz" -C /usr/local/bin goreleaser; \
    get "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT}/golangci-lint-${GOLANGCI_LINT}-linux-${arch}.tar.gz" \
      "golangci-lint-${GOLANGCI_LINT}-checksums.txt" "$GOLANGCI_LINT_SUMS" "golangci-lint-${GOLANGCI_LINT}-linux-${arch}.tar.gz"; \
    tar -xzf "golangci-lint-${GOLANGCI_LINT}-linux-${arch}.tar.gz" -C /usr/local/bin --strip-components=1 \
      "golangci-lint-${GOLANGCI_LINT}-linux-${arch}/golangci-lint"; \
    rm -rf /tmp/*; \
    terraform version; tofu version; goreleaser --version | head -1; golangci-lint version
