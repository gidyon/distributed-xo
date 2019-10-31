FROM alpine
LABEL maintainer="gideonhacer@gmail.com"
RUN apk update && \
   apk add ca-certificates && \
   update-ca-certificates && \
   rm -rf /var/cache/apk/*
COPY game /
COPY dist /dist
EXPOSE 443
ENTRYPOINT ["/game"]
CMD ["-env"]