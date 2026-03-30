FROM fluent/fluentd:v1.16-debian-1

USER root
RUN gem install elasticsearch -v '~> 8.0' --no-document \
    && gem install fluent-plugin-elasticsearch -v '~> 5.4' --no-document \
    && mkdir -p /var/log/fluentd-buffers \
    && chown fluent:fluent /var/log/fluentd-buffers
USER fluent

COPY fluent.conf /fluentd/etc/fluent.conf
