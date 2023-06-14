
我们使用基于 RBAC 模型的权限系统。

## 1. model.conf

参考 `./rbac_model.conf`

## 2. policy.conf

使用 gormadaptar 将策略保存在数据库中。

2.1 需要默认添加的策略配置：

```
p, hall, *, *
p, proj_admin, proj, create
p, guild_admin, guild, create
p, proj_admin, proj, close
p, guild_admin, guild, close

g, 0x183f09c3ce99c02118c570e03808476b22d63191, hall
```

市政厅成员是超级管理员，可以做任意操作。

2.2 需要动态添加的策略配置 示例：

```
p, proj_sponsor_1, proj_1, modify
p, proj_member_1, proj_1, modify

g, 0xc13..1283 proj_sponsor_1
g, 0x179..d345 proj_member_1
```

## 3. 测试请求

```
0x183f09c3ce99c02118c570e03808476b22d63191, proj, create # true
0x183f09c3ce99c02118c570e03808476b22d63191, proj_1, modify # true
0xc1ee7cb74583d1509362467443c44f1fca981283, proj, create # false
```
