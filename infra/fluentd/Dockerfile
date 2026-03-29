FROM fluent/fluentd:v1.16-1

USER root
RUN gem install fluent-plugin-elasticsearch --no-document
USER fluent

COPY fluent.conf /fluentd/etc/fluent.conf
