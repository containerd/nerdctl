# Multi-platform compose demo

- Make sure QEMU is configured, see [`../../docs/multi-platform.md`](../../docs/multi-platform.md)
- Run `nerdctl compose up -d`
- Open http://localhost:8080 , and confirm that "System" is ppc64le
- Open http://localhost:8081 , and confirm that "System" is s390x
