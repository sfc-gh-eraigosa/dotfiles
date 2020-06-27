#!/bin/sh

alias docker-ucp='docker --tlsverify --tlscacert="$(pwd)/ca.pem" --tlscert="$(pwd)/cert.pem" --tlskey="$(pwd)/key.pem" -H="tcp://bti-ucp.vse.rdlabs.hpecorp.net:443"'

