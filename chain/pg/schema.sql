-- Version: 1.01
-- Description: Create type round
CREATE TYPE drand_round AS
    (round bigint, signature bytea, previous_sig bytea);

-- Version: 1.02
-- Description: Create type round offset
CREATE TYPE drand_round_offset AS
    (round_offset bigint);

-- Version: 1.03
-- Description: Create function drand_tablesize
CREATE FUNCTION drand_tablesize(tableName char) RETURNS bigint AS $$
DECLARE ret bigint;
BEGIN
    EXECUTE (FORMAT('SELECT COUNT(*) FROM %I', tableName)) INTO ret;
    RETURN ret;
END;
$$ LANGUAGE plpgsql;

-- Version: 1.04
-- Description: Create function drand_insertround
CREATE FUNCTION drand_insertround(tableName char, round bigint, signature bytea, previous_sig bytea) RETURNS VOID AS $$
BEGIN
    EXECUTE FORMAT('CREATE TABLE IF NOT EXISTS %I (
        round        BIGINT NOT NULL CONSTRAINT %1$I_pk PRIMARY KEY,
        signature    BYTEA  NOT NULL,
        previous_sig BYTEA  NOT NULL
    )', tableName);

    EXECUTE FORMAT('INSERT INTO %I
        (round, signature, previous_sig)
    VALUES
        (%L, %L, %L)
    ON CONFLICT DO NOTHING', tableName, round, signature, previous_sig);
END;
$$ LANGUAGE plpgsql;

-- Version: 1.05
-- Description: Create function drand_getlastround
CREATE OR REPLACE FUNCTION drand_getlastround(tableName char) RETURNS drand_round AS $$
DECLARE ret drand_round;
BEGIN
    EXECUTE (FORMAT('SELECT round, signature, previous_sig FROM %I
                    ORDER BY round DESC
                    LIMIT 1',
                    tableName)) INTO ret;
    RETURN ret;
END;
$$ LANGUAGE plpgsql;

-- Version: 1.06
-- Description: Create function drand_getround
CREATE FUNCTION drand_getround(tableName char, round bigint) RETURNS drand_round AS $$
DECLARE ret drand_round;
BEGIN
    EXECUTE (FORMAT('SELECT round, signature, previous_sig FROM %I
                    WHERE round=%L
                    LIMIT 1',
                    tableName, round)) INTO ret;
    RETURN ret;
END;
$$ LANGUAGE plpgsql;

-- Version: 1.07
-- Description: Create function drand_deleteround
CREATE FUNCTION drand_deleteround(tableName char, round bigint) RETURNS VOID AS $$
BEGIN
    EXECUTE (FORMAT('SELECT round, signature, previous_sig FROM %I
                    WHERE round=%L
                    LIMIT 1',
                    tableName, round));
END;
$$ LANGUAGE plpgsql;

-- Version: 1.08
-- Description: Create function drand_getroundposition
CREATE FUNCTION drand_getroundposition(tableName char, round_num int) RETURNS drand_round_offset AS $$
DECLARE ret drand_round_offset;
BEGIN
    EXECUTE (FORMAT('SELECT round_offset FROM (
            SELECT round, row_number() OVER(ORDER BY round ASC) AS round_offset
            FROM %I
            ORDER BY round ASC
        ) result WHERE round=%L',
        tableName, round_num)) INTO ret;
    RETURN ret;
END;
$$ LANGUAGE plpgsql;

-- Version: 1.09
-- Description: Create function drand_getfirstround
CREATE FUNCTION drand_getfirstround(tableName char) RETURNS drand_round AS $$
DECLARE ret drand_round;
BEGIN
    EXECUTE (FORMAT('SELECT round, signature, previous_sig FROM %I
                    ORDER BY round ASC
                    LIMIT 1',
                    tableName)) INTO ret;
    RETURN ret;
END;
$$ LANGUAGE plpgsql;

-- Version: 1.10
-- Description: Create function drand_getoffsetround
CREATE FUNCTION drand_getoffsetround(tableName char, r_offset bigint) RETURNS drand_round AS $$
DECLARE ret drand_round;
BEGIN
    EXECUTE (FORMAT('SELECT round, signature, previous_sig FROM %I
					ORDER BY round ASC OFFSET %L LIMIT 1',
                     tableName, r_offset)) INTO ret;
    RETURN ret;

END;
$$ LANGUAGE plpgsql;
