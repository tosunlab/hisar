FROM golang:1.26 AS build
WORKDIR /src
COPY core core
COPY server server
RUN cd server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /hisar-server ./cmd/hisar-server
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /hisar-server /usr/local/bin/hisar-server
COPY --from=build --chown=65532:65532 /data /data
ENV HISAR_ADDR=:8080 HISAR_SECRETS_FILE=/data/hisar-secrets.json
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["hisar-server"]
CMD ["serve"]
