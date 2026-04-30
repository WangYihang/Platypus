-- 000027_host_public_ip_dual.up.sql — split the single public_ip column
-- introduced in 000024 into per-family public_ipv4 / public_ipv6
-- columns. The agent now probes o-o.myaddr.l.google.com over both
-- transports in parallel so a dual-stack host can report both
-- addresses (different geo, different ISP). Keeping public_ip on the
-- table as a back-compat read for older clients; the server populates
-- it as a copy of whichever family came back from the agent.
--
-- All three columns stay nullable; the next agent link-up backfills
-- whatever the host can actually reach.

ALTER TABLE hosts ADD COLUMN public_ipv4 TEXT;
ALTER TABLE hosts ADD COLUMN public_ipv6 TEXT;
