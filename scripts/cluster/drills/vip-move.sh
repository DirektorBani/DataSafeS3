#!/usr/bin/env bash
# Drill: stop keepalived on current VIP owner; expect backup to take VIP.
set -euo pipefail
echo "On VIP owner: systemctl stop keepalived"
echo "On peer: ip addr show | grep <VIP>"
echo "Expect VIP moves within advert_int*fall (~seconds)."
echo "SLA target (product): VIP recovery without manual script."
