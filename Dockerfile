FROM ghcr.io/jenkins-x/jx-boot:latest

ENTRYPOINT ["jx-application"]

COPY ./build/linux/jx-application /usr/bin/jx-application