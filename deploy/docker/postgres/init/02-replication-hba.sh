#!/bin/sh
# Allow streaming replication from standby containers on the compose network.
set -e
{
  echo "host replication all 0.0.0.0/0 scram-sha-256"
  echo "host replication all 0.0.0.0/0 md5"
} >> "$PGDATA/pg_hba.conf"