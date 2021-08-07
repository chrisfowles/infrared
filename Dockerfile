FROM golang:1.16.0-buster AS builder
LABEL stage=intermediate
COPY . /infrared
WORKDIR /infrared/cmd/infrared
ENV GO111MODULE=on
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a --installsuffix cgo -v -tags netgo -ldflags '-extldflags "-static"' -o /main .

FROM scratch
LABEL maintainer="Hendrik Jonas Schlehlein <hendrik.schlehlein@gmail.com>"
WORKDIR /
COPY --from=builder /main ./
ENTRYPOINT [ "./main" ]