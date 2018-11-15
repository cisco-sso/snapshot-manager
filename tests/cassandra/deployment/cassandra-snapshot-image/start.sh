#!/bin/bash
sed -i 's/cluster_name: cassandra/cluster_name: snapshot-cassandra/' /etc/cassandra/cassandra.yaml
#rm -rf /var/lib/cassandra/data/system*
ls /var/lib/cassandra/data/system/local-**/*
rm -rf /var/lib/cassandra/data/system/local-**/mc-*-big-Data.db
docker-entrypoint.sh cassandra -f
