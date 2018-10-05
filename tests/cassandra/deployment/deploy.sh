#!/bin/bash

#export DEBUG="echo "

export KUBECTL="${DEBUG}kubectl"

for f in deploy-*-*; do
    case $f in
        *.yaml)
            $KUBECTL create -f $f 
            ;;
        *.bash)
            ./$f 
            ;;
    esac
done
