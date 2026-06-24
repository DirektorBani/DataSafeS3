-- Clear recent_items cache rows (legacy id formats may be incompatible with Postgres TEXT rules).
DELETE FROM recent_items;
