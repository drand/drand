#!/bin/bash
# set -x

### This script allows you to deploy our drand website on your domain
## There are 2 modes: interactive and flag based

if [ $# -eq 0 ]
  then
    echo Before starting make sure that your SSH setup is compatible with: https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key
    echo What is your user name ?
    read USER
    echo What is your website\'s URL ? \(https://drand.io/\)
    read URL
    #format the /
    URL="$(sed 's/\//\\\//g' <<<$URL)"
    echo What is the host name you want to deploy to ?
    read HOST
    echo What is the path of the destination directory ? \(/var/www/html/\)
    read DIR
    echo Let\'s deploy...

    #replace url in config.toml
    VAR1="https:\/\/drand.io\/"
    sed -i -e "s/$VAR1/$URL/g" "$PWD/config.toml"

    hugo && rsync -Paivz --delete public/ ${USER}@${HOST}:${DIR}

    exit 0
  else
    while [ ! $# -eq 0 ]
    do
	     case "$1" in
		       --user)
              export USER=$2
	            ;;
		       --host)
              export HOST=$2
	            ;;
          --url)
              export URL=$2
	            ;;
          --dir)
             export DIR=$2
             ;;
         --help | -h)
            echo "Before starting make sure that your SSH setup is compatible with: https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key"
            echo "Args like --user user --host host --dir dir --url url"
            echo "or start without args to go into interactive mode"
            exit
            ;;
	     esac
	     shift
    done
    #format the /
    URL="$(sed 's/\//\\\//g' <<<$URL)"
    echo Let\'s deploy...
    #replace url in config.toml ?
    VAR1="https:\/\/drand.io\/"
    sed -i -e "s/$VAR1/$URL/g" "$PWD/config.toml"

    hugo && rsync -Paivz --delete public/ ${USER}@${HOST}:${DIR}

    exit 0
fi
