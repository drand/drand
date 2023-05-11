-- Version: 1.01
-- Description: Create beacons table
CREATE TABLE beacons (
    id    SERIAL,
    name  TEXT    NOT NULL,

    PRIMARY KEY (ID)
);

-- Version: 1.02
-- Description: Create beacons table index
CREATE UNIQUE INDEX index_beacons_name ON beacons (name);

-- Version: 1.03
-- Description: Create beacon details table
CREATE TABLE beacon_details (
    beacon_id     INT     NOT NULL,
    round         BIGINT  NOT NULL,
    signature     BYTEA   NOT NULL,
    previous_sig  BYTEA       NULL,

    CONSTRAINT pk_beacon_id_round PRIMARY KEY (beacon_id, round),
    CONSTRAINT fk_beacon_id FOREIGN KEY (beacon_id) REFERENCES beacons(id) ON DELETE CASCADE
);

-- Version: 1.04
-- Description: Drop the previous_sig column
ALTER TABLE beacon_details
    DROP COLUMN previous_sig;
