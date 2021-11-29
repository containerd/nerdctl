#Troubleshooting

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Enable dbus user session](#enable-dbus-user-session)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Enable dbus user session

```
FATA[0000] failed to create shim task: OCI runtime create failed: runc create failed: unable to start container process: unable to apply cgroup configuration: unable to start unit "nerdctl-7bda4abaa1f006ab9feeb98c06953db43f212f1c0aaf658fb8a88d6f63dff9f9.scope" (properties [{Name:Description Value:"libcontainer container 7bda4abaa1f006ab9feeb98c06953db43f212f1c0aaf658fb8a88d6f63dff9f9"} {Name:Slice Value:"user.slice"} {Name:Delegate Value:true} {Name:PIDs Value:@au [1154]} {Name:MemoryAccounting Value:true} {Name:CPUAccounting Value:true} {Name:IOAccounting Value:true} {Name:TasksAccounting Value:true} {Name:DefaultDependencies Value:false}]): Permission denied: unknown
```

Don't panic. Enabling dbus user session is typically needed for using systemd and cgroup v2. Otherwise runc may fail with an error like the one above

Fixing it?:

```
sudo apt-get install -y dbus-user-session

systemctl --user start dbus
```
