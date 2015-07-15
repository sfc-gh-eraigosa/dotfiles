#!/bin/bash
if [ -f ~/.sshd.env ] ; then
    . ~/.sshd.env
fi
if [ "${SSHD_LOGIN}" = "true" ] ; then
    sudo /usr/sbin/sshd -D &
else
    echo "SSHD_LOGIN not true : $SSHD_LOGIN"
fi

