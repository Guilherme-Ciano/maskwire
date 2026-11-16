# 0006. Checksum-validated detectors instead of an NER model

## Context

The proxy sits on the hot path of every request. NER models add tens of milliseconds and a Python or ONNX runtime,
and still produce false positives on numbers.

## Decision

Detection uses single-pass scanners with real validators: CPF, CNPJ (including the alphanumeric format), PIS, PT
NIF, ES DNI/NIE, DE Steuer-ID, FR NIR, IT codice fiscale, NL BSN, PL PESEL, BE NN, AT SVNR, SE personnummer, FI
HETU, IE PPSN, UK NINO, IBAN mod-97, Luhn for cards, libphonenumber for phones, plus e-mail and IP. Bare digit runs
that pass a checksum by chance get a lower score than formatted or keyword-adjacent matches, and policies can set
`min_score`.

## Consequences

- Clean text is scanned at hundreds of MB/s per core with zero allocations.
- Person names, addresses and free-text health data are not detected. The README says so, and names are a
  separate roadmap item that will not use a model on the hot path by default.
