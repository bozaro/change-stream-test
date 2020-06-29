#
# Services for test environment
#
FROM ubuntu:bionic

ARG mongo_version=4.0.16

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
  && apt-get install -y \
  tzdata gnupg ca-certificates apt-transport-https supervisor sudo apt-utils software-properties-common netcat-openbsd eatmydata \
  && rm -rf /var/lib/apt/lists/*

# Mongo DB
RUN apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 9DA31620334BD75D9DCB49F368818C72E52529D4
RUN echo "deb [ arch=amd64 ] https://repo.mongodb.org/apt/ubuntu bionic/mongodb-org/4.0 multiverse" | tee /etc/apt/sources.list.d/mongodb-org.list
RUN apt-get update \
    && apt-get install -y mongodb-org=$mongo_version mongodb-org-tools=$mongo_version \
    mongodb-org-mongos=$mongo_version mongodb-org-server=$mongo_version mongodb-org-shell=$mongo_version \
    && rm -rf /var/lib/apt/lists/*

COPY supervisor/ /etc/supervisor/conf.d/
COPY scripts/ /opt/bin/

ENV PATH=$PATH:/opt/bin

RUN addgroup --system --gid 1000 ubuntu
RUN adduser --system --uid 1000 --gid 1000 ubuntu

EXPOSE \
  # Mongo port
  27017/tcp \
  27018/tcp \
  27019/tcp \
  28000/tcp \
  29000/tcp

ENTRYPOINT ["docker-init"]
CMD ["/bin/sleep", "infinity"]
