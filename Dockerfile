FROM golang:1.21-alpine
RUN apk add git

COPY . /home/src
WORKDIR /home/src
RUN go build -o /bin/action ./

ENTRYPOINT [ "/bin/action" ]