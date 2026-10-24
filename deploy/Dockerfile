FROM --platform=$BUILDPLATFORM golang:1.26.10@sha256:5c3f3f28a273c9b61698b2d34f18375929790ab1f1d3835d032d21e2b31eae0c AS admin-wasm
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY core core
COPY server server
RUN cd server && GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o /admin-wasm/hisar.wasm ./cmd/hisar-admin-wasm \
    && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /admin-wasm/

FROM --platform=$BUILDPLATFORM node:24@sha256:14078204b6bb268c107ceff6d3c43f45a888105e5c2b18b14c32b3409d93a1b0 AS admin
WORKDIR /src/admin
COPY admin/package.json admin/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY admin/index.html admin/tsconfig.json admin/vite.config.ts admin/playwright.config.ts ./
COPY admin/src src
COPY admin/end-to-end end-to-end
COPY --from=admin-wasm /admin-wasm wasm
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.10@sha256:5c3f3f28a273c9b61698b2d34f18375929790ab1f1d3835d032d21e2b31eae0c AS build
ENV GOTOOLCHAIN=local
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY core core
COPY server server
COPY --from=admin /src/server/internal/api/adminpage/dist server/internal/api/adminpage/dist
ARG VERSION=dev
RUN cd server && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /hisar-server ./cmd/hisar-server
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /hisar-server /usr/local/bin/hisar-server
COPY --from=build --chown=65532:65532 /data /data
ENV HISAR_ADDR=:8080 HISAR_SECRETS_FILE=/data/hisar-secrets.json
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["hisar-server"]
CMD ["serve"]
