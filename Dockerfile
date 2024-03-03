FROM alpine:3.19.0

COPY external-dns-unbound /
COPY default_config.yml /config.yml

CMD [ "/external-dns-unbound" ]

EXPOSE 8080
