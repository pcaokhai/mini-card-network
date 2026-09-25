-- docs/03-iso8583-interface-spec.md §8 codes the baseline never inserted. tran_log.response_code is
-- an FK to this table, so any of these answered to the acquirer made the issuer's log update fail
-- (seen with RC 62 on a blocked card, which v1 uses for every restricted card status).
INSERT INTO response_code VALUES
  ('06','Error','ERROR'),
  ('10','Partial approval','APPROVED'),
  ('17','Customer cancellation','DECLINED'),
  ('62','Restricted card','DECLINED'),
  ('68','Response received too late','ERROR'),
  ('95','Reconciliation error','ERROR')
ON CONFLICT (code) DO NOTHING;
