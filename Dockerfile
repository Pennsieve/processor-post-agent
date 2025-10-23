FROM pennsieve/pennsieve-agent:1.4.5

RUN apk update && apk upgrade

WORKDIR /service/

RUN cp /root/pennsieve /service/

ENV PATH="${PATH}:/service"

# install Go
RUN wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz

RUN  rm -rf /usr/local/go && tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz

ENV PATH="${PATH}:/usr/local/go/bin"

# cleanup
RUN rm -f go1.21.0.linux-amd64.tar.gz

COPY . ./
RUN GOARCH=amd64 GOOS=linux go build -o /service/bootstrap main.go

RUN chmod +x agent.sh
RUN chown 1000:1000 agent.sh

RUN apk --no-cache add curl

RUN mkdir /mnt/efs
RUN mkdir -p /service/logs
RUN mkdir -p data

ENTRYPOINT [ "/service/bootstrap" ]