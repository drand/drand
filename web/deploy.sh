#!/bin/bash
# set -x

### This script allows you to deploy our drand website on your domain
## There are 2 modes: interactive and flag based

if [ $# -eq 0 ]
  then
    echo Starting the deployment in *interactive* mode. Run sh deploy.sh --help for more information about the different modes.
    echo Before beginning, make sure that your SSH setup is compatible with: https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key.
    read -p "Continue (y/n)? " answer
      case ${answer:0:1} in
        y|Y )
        ;;
        * )
          exit 0
        ;;
      esac
    echo What is your website\'s URL ? \(ex: https://drand.io\)
    read URL
    #format the /
    URL="$(sed 's/\//\\\//g' <<<$URL)"
    echo What is your user name ? \(this parameter is optional depending on your SSH setup\)
    read USER
    if ! [ -z "$USER" ]
    then
      USER="$USER@"
    fi
    echo What is the host name ?
    read HOST
    echo What is the path on the server of the destination directory ? \(ex: /var/www/html/\)
    read DIR
    echo Let\'s deploy...

    #replace url in config.toml
    sed -i -e "1s/.*/baseURL\=\"$URL\"/g" "$PWD/config.toml"

    hugo && rsync -Paivz --delete public/ ${USER}${HOST}:${DIR}

    exit 0
  else
    while [ ! $# -eq 0 ]
    do
	     case "$1" in
		       --user)
              export USER=$2
              echo user
              echo $USER
              if [ -z "$USER" ] || [[ $USER == --* ]]
              then
                echo Bad user format
                exit 1
              fi
	            ;;
		       --host)
              export HOST=$2
              echo host
              echo $HOST
              if [ -z "$HOST" ] || [[ $HOST == --* ]]
              then
                echo Bad host format
                exit 1
              fi
	            ;;
          --url)
              export URL=$2
              if [ -z "$URL" ] || [[ $URL == --* ]]
              then
                echo Bad URL format
                exit 1
              fi
	            ;;
          --dir)
             export DIR=$2
             if [ -z "$DIR" ] || [[ $DIR == --* ]]
             then
               echo Bad dir format
               exit 1
             fi
             ;;
         --help | -h)
            echo "This script can be used in either interactive or non-interactive mode.
In interactive mode you will be asked for the parameters one at a time, whereas in the non-interactive mode, you need to specify them all at once, by using flags (i.e., \"--user \$USER\").
The parameters that we ask for are USER, HOST, DIR and URL.
USER and HOST are the variables you used during your SSH setup, thus you need to sure that your SSH setup is compatible with: https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key. ULR is the address you want to deploy to, such as https://example.com, and DIR is the path of the destination on the server.
To start in interactive mode, run the script without any argument. To use the script in non-interactive mode, you have to specify every flag with the corresponding value, i.e., sh deploy.sh --user \$USER --host \$HOST --dir \$DIR --url \$URL.
Important note: the USER parameter may be optional depending on your SSH configuration. To skip it, simply press enter when asked for it in the interactive mode, or omit the flag \"--user\" in the non-interactive mode."
            exit
            ;;
	     esac
	     shift
    done
    echo Starting the deployment in non-interactive mode:
    #format the /
    URL="$(sed 's/\//\\\//g' <<<$URL)"
    #replace url in config.toml
    sed -i -e "1s/.*/baseURL\=\"$URL\/\"/g" "$PWD/config.toml"
    if ! [ -z "$USER" ]
    then
      USER="$USER@"
    fi

    hugo && rsync -Paivz --delete public/ ${USER}${HOST}:${DIR}

    exit 0
fi
