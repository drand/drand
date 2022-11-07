package pg

import (
	"context"
	"sync"

	"github.com/jmoiron/sqlx"
)

var bootstrapFuncs sync.Once

// Load all the required functions in the database, or ignore their creation.
//
//nolint:funclen // working as intended
func doBootstrapFuncs(ctx context.Context, db *sqlx.DB, isTest bool) (err error) {
	queries := []string{
		//language=postgresql
		`DO
$$
    BEGIN
        IF NOT EXISTS (SELECT *
                       FROM pg_type typ
                                INNER JOIN pg_namespace nsp
                                           ON nsp.oid = typ.typnamespace
                       WHERE nsp.nspname = current_schema()
                         AND typ.typname = 'drand_round') THEN
            CREATE  TYPE drand_round AS (round bigint, signature bytea, previous_sig bytea);
        END IF;
    END;
$$
LANGUAGE plpgsql;`,
		//language=postgresql
		`DO
$$
    BEGIN
        IF NOT EXISTS (SELECT *
                       FROM pg_type typ
                                INNER JOIN pg_namespace nsp
                                           ON nsp.oid = typ.typnamespace
                       WHERE nsp.nspname = current_schema()
                         AND typ.typname = 'drand_round_offset') THEN
            CREATE  TYPE drand_round_offset AS (round_offset bigint);
        END IF;
    END;
$$
LANGUAGE plpgsql;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_maketable(tableName char)
    RETURNS VOID
	LANGUAGE plpgsql
AS
$$
    BEGIN
		EXECUTE format('CREATE TABLE IF NOT EXISTS %I (
			round        BIGINT NOT NULL CONSTRAINT %1$I_pk PRIMARY KEY,
			signature    BYTEA  NOT NULL,
			previous_sig BYTEA  NOT NULL
		)', tableName);
	END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_tablesize(tableName char)
	RETURNS bigint
	LANGUAGE plpgsql
AS
$$
    DECLARE ret bigint;
    BEGIN
        EXECUTE (format('SELECT COUNT(*) FROM %I', tableName)) INTO ret;
        RETURN ret;
	END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_insertround(tableName char, round bigint, signature bytea, previous_sig bytea)
	RETURNS VOID
	LANGUAGE plpgsql
AS
$$
    BEGIN
        EXECUTE format('INSERT INTO %I
			(round, signature, previous_sig)
		VALUES
			(%L, %L, %L)
		ON CONFLICT DO NOTHING', tableName, round, signature, previous_sig);
	END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getlastround(tableName char)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     ORDER BY round DESC
                     LIMIT 1',
                     tableName)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getround(tableName char, round bigint)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     WHERE round=%L
                     LIMIT 1',
                     tableName, round)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_deleteround(tableName char, round bigint)
    RETURNS VOID
    LANGUAGE plpgsql
AS
$$
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     WHERE round=%L
                     LIMIT 1',
                     tableName, round));
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getroundposition(tableName char, round_num int)
    RETURNS drand_round_offset
    LANGUAGE plpgsql
AS
$$
DECLARE ret drand_round_offset;
BEGIN
    EXECUTE (format('SELECT round_offset FROM (
   	        SELECT round, row_number() OVER(ORDER BY round ASC) AS round_offset
		    FROM %I
            ORDER BY round ASC
        ) result WHERE round=%L',
        tableName, round_num)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getfirstround(tableName char)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     ORDER BY round ASC
                     LIMIT 1',
                     tableName)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getoffsetround(tableName char, r_offset bigint)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
					ORDER BY round ASC OFFSET %L LIMIT 1',
                     tableName, r_offset)) INTO ret;
    RETURN ret;

END;
$$;`,
	}

	runQueries := func() {
		for _, query := range queries {
			_, err = db.DB.ExecContext(ctx, query)
			if err != nil {
				return
			}
		}
	}

	if isTest {
		runQueries()
	} else {
		bootstrapFuncs.Do(runQueries)
	}

	return err
}
