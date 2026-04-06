-- Migration 012 down: Remove bid leveling analysis table
DROP INDEX IF EXISTS idx_bid_analyses_project;
DROP INDEX IF EXISTS idx_bid_analyses_org;
DROP TABLE IF EXISTS bid_analyses;
