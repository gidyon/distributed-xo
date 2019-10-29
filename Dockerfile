FROM scratch
LABEL maintainer="gideonhacer@gmail.com"
COPY game /
EXPOSE 443
ENTRYPOINT [ "/game" ]
CMD [ "-env" ]