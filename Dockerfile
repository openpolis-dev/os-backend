# First stage: build the binary
FROM golang:1.20 AS build

WORKDIR /go/src/

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/apiserver ./cmd/apiserver.go

# Second stage: build the deliver image
FROM alpine:latest

COPY --from=build /go/bin/apiserver /usr/local/bin/apiserver
COPY --from=build /go/src/rbac_model.conf /superapp-backend/conf/rbac_model.conf

WORKDIR /superapp-backend/
ENTRYPOINT [ "/usr/local/bin/apiserver", "-casbin-model", "/superapp-backend/conf/rbac_model.conf", "-config", "/superapp-backend/conf/config.yml" ]