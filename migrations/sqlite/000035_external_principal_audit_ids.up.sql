-- SQLite does not enforce VARCHAR lengths. Fresh databases use the widened
-- declaration in 000000_init; existing databases already accept these ids.
SELECT 1;
