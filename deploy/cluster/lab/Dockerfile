FROM alpine:3.20

RUN apk add --no-cache \
      openssh sudo curl ca-certificates python3 py3-pip \
      haproxy keepalived nfs-utils \
      postgresql postgresql-contrib \
      etcd \
      procps iproute2 iputils \
      bash shadow \
    && python3 -m pip install --break-system-packages 'patroni[etcd3]' \
    && mkdir -p /var/run/sshd /var/lib/datasafe /etc/datasafe /etc/ssh \
    && ssh-keygen -A \
    && adduser -D -s /bin/bash datasafes3 \
    && echo 'datasafes3 ALL=(root) NOPASSWD:ALL' > /etc/sudoers.d/datasafes3-lab \
    && chmod 440 /etc/sudoers.d/datasafes3-lab

# Lab-only NOPASSWD:ALL — production Apply uses scoped sudoers template.

COPY lab-entrypoint.sh /usr/local/bin/lab-entrypoint.sh
RUN chmod +x /usr/local/bin/lab-entrypoint.sh

EXPOSE 22 2379 2380 5432 8008 8080 9000 8404

ENTRYPOINT ["/usr/local/bin/lab-entrypoint.sh"]
