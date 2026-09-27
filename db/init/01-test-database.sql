-- Runs once, on first container start, before any application migration.
-- Integration tests get their own database so a failed test run cannot damage dev data.
CREATE DATABASE nimbus_test OWNER nimbus;
