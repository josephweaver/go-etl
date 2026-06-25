# SSH Transport Epic

Goal:
Allow controller Transport implementations to execute commands and copy files over SSH.

Non-goals:
- Full HPCC execution
- Persistent SSH session pooling
- Credential management UI

Slices:
- [ ] 001 Test strategy
- [ ] 002 Command runner boundary
- [ ] 003 Execute
- [ ] 004 CopyInto
- [ ] 005 Config wiring
- [ ] 006 Integration smoke