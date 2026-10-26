-- Keep the fallback and do not restore the broken trigger on rollback.
-- Dropping a populated default partition would destroy audit history.
SELECT 1;
