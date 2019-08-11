#!/bin/bash
# set -x

### This script allows to deploy the proposed drand website to the given address

echo Before starting, make sure that your ssh setup is compatible with what is done at https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key
echo What is the host name you want to deploy to ?
read HOST
echo Confirmation check: host name is $HOST
echo What is your domain URL ?
read URL
echo Confirmation check: url is $URL
echo What is your user name ?
read USER
echo Confirmation check: user name is $USER
echo What is the path of the destination directory ? (/var/www/html/)
read DIR
echo Confirmation check: path is $DIR
#read -r -p "${1:-Please make sure that your ssh setup is compatible with https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key. If not, would you like us to do the setup for you ? [y/n]} " response
#case "$response" in
#  [yY])
#  echo Creating new ssh key and registering it on remote host...

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
#  ;;
#  *)
#  ;;
#esac
echo Let\'s deploy...

#replace url in config.toml ?
var1 = "https://drand.io/"
sed -i -e "s/$var1/$URL/g" /config.toml

hugo && rsync -Paivz --delete public/ ${USER}@${HOST}:${DIR}

exit 0
