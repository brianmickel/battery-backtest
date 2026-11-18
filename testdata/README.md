# Synthetic verification fixtures

These hand-authored rows describe an imaginary market and contain no provider exports or credentials. Go and TypeScript tests share this directory.

| File | Meaning |
| --- | --- |
| two-price.json | 1 MWh charged at 10 then discharged at 100 earns 90 before costs. |
| variable-gap.json | 15 and 30 covered minutes separated by a 15-minute gap. Negative prices are valid. |
| midnight.json | One 30-minute interval divides equally across two market dates. |
| dst-fall.json | Adjacent UTC intervals retain the repeated local hour and changing offsets. |
| half-hour.json | A single 30-minute price interval. |
| half-hour-ledger.json | Charging at 100 MW for 30 minutes draws 50 MWh. Chart height is 100 MW. |
| single-request.json | Credential-free custom configuration with explicit zero initial SOC and degradation. |
| compare-request.json | Ordered idle/oracle variations; zero power remains idle. |

Date intervals are half-open. A provider header is supplied separately by the caller. JSON file names describe inputs, not golden outputs of the old implementation.
