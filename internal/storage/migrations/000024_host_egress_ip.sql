-- 000024_host_egress_ip.up.sql — record both the server-derived egress
-- IP (whatever the WS upgrade peered from on TCP) and the agent-derived
-- public IP (DNS TXT lookup of o-o.myaddr.l.google.com) on each host
-- row. Without these, an agent behind NAT only ever shows its private
-- LAN IP in the host list, which tells the operator nothing about
-- where on the internet the box actually is.
--
-- Both columns nullable; populated on the next agent link-up — no
-- backfill needed. They diverge meaningfully under mesh relay
-- (egress_ip will be the relay, public_ip the original agent), so we
-- keep them as separate columns rather than collapsing them.

ALTER TABLE hosts ADD COLUMN egress_ip TEXT;
ALTER TABLE hosts ADD COLUMN public_ip TEXT;
