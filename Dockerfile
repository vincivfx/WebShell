FROM alpine:latest

RUN apk add libc6-compat

COPY build/ app/

EXPOSE 8080

WORKDIR /app

ENTRYPOINT [ "./webshell" ]
