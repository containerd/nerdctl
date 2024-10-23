FROM golang:bookworm
RUN go install github.com/caddyserver/xcaddy/cmd/xcaddy@v0.4.2
RUN "$(go env GOPATH)"/bin/xcaddy build --with github.com/caddyserver/cache-handler --with github.com/darkweak/storages/nuts/caddy
COPY ./Caddyfile.conf /config/Caddyfile.conf
CMD ["./caddy", "run", "--config", "/config/Caddyfile.conf", "--adapter", "caddyfile"]
