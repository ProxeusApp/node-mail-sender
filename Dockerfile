FROM alpine
ARG BIN_NAME=node-mail-sender
WORKDIR /app
COPY /artifacts/$BIN_NAME /app/node
EXPOSE 8013
ENTRYPOINT ["./node"]
