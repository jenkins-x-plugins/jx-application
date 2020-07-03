FROM centos:7

RUN yum install -y git

ENTRYPOINT ["jx-application"]

COPY ./build/linux/jx-application /usr/bin/jx-application