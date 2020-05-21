# Changelog

## 1.0.1

- Use `ListTasks` and `DescribeTasks` to get list of running tasks
instead of relying on `RunningTasksCount` to be able
to handle better tasks in `DEACTIVATING`, `DEPROVISIONING`, etc. states

- Update dependencies
