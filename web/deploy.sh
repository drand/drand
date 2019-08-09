#!/bin/bash
# set -x

# This script allows to deploy the proposed drand website to the given address

#HOST=127.0.0.1
#DIR=/var/www/html/

echo HOST name ?
read HOST
echo Host name is $HOST
echo User name ?
read USER
echo User name is $USER
echo Dir path ?
read DIR
echo Dir path is $DIR
read -r -p "${1:-wanna do ssh ? [y/n]} " response
case "$response" in
  [yY])
  echo yes
  true
  ;;
  *)
  echo no
  false
  ;;
esac


# need to create ssh key
#var rep = pwd

#cd && mkdir .ssh & cd .ssh
#ssh-keygen -t rsa -q -C "For SSH" -f rsa_id
#cat >> config <<EOF
#Host HOST
#Hostname HOST
#Port 22
#User USER
#IdentityFile ~/.ssh/rsa_id
#EOF

#ssh-copy-id -i rsa_id.pub USER@HOST.com
#ssh user@host

#cd rep

#replace baseURL in config.toml ?
#var1 = https://drand.io/
#var2 = ARGS
#sed -i -e 's/'"$var1"'/'"$var2"'/g' /config.toml

#hugo && rsync -avz --delete public/ ${HOST}:${DIR}
#-Paivz vs -avz

exit 0
