-- +migrate Down
DROP SCHEMA IF EXISTS eigenda CASCADE;

-- +migrate Up
CREATE SCHEMA eigenda;

CREATE TABLE eigenda.data_ref (
    key VARCHAR PRIMARY KEY, header_hash VARCHAR, blob_index BIGINT
);