FROM golang:1.21
RUN apt-get update && apt-get install -y git diffstat

COPY . /home/src
WORKDIR /home/src
RUN go build -o /bin/action ./

ENTRYPOINT [ "/bin/action" ]