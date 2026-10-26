CREATE TABLE auth.rate_windows(scope text NOT NULL,client text NOT NULL,window_start timestamptz NOT NULL,count integer NOT NULL,PRIMARY KEY(scope,client,window_start));
CREATE INDEX rate_window_expiry ON auth.rate_windows(window_start);
