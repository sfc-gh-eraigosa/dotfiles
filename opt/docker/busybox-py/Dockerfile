# teting out busybox
#
# VERSION               0.0.1

FROM progrium/busybox
MAINTAINER Edward Raigosa (wenlock) <wenlock@hp.com>

ENV ROOT_PASS changeme
ENV NOTVISIBLE "in users profile"

RUN opkg-install curl bash git \
    openssh-server python python-dev python-distribute python-pip
RUN pip install virtualenv
RUN pip install tox
RUN pip install cherrypy
RUN pip install flask
RUN mkdir /var/run/sshd
#TODO: fix, need to find a way to get chpasswd
RUN echo 'root:$ROOT_PASS' | chpasswd
RUN sed -i 's/PermitRootLogin without-password/PermitRootLogin yes/' /etc/ssh/sshd_config
RUN /usr/bin/ssh-keygen -A ; \
    mkdir -p /var/empty/sshd/etc; \
    cd /var/empty/sshd/etc; \
    ln -s /etc/localtime localtime

# SSH login fix. Otherwise user is kicked off after login
RUN sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd
RUN echo "export VISIBLE=now" >> /etc/profile

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D"]
