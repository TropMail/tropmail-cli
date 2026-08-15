FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
  && adduser -D -u 10001 tropmail

# GoReleaser stages each platform's artifacts under $TARGETPLATFORM/.
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/tropmail /usr/local/bin/tropmail

USER tropmail
WORKDIR /home/tropmail

ENTRYPOINT ["/usr/local/bin/tropmail"]
