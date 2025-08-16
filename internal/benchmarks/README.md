# Comparison of znkr.io/diff with other implementations

Run this command to produce the comparison:

```
go test -count 10 -bench=. . | benchstat -col /impl -row /name -
```
