# ddns_sp

简单ddns,仅支持cloudflare.

启动之后尝试开启一个长连接代表.然后连接断开之后尝试检查IP是否变化,然后更新dns.

## 问题

go语言消耗太大了,如果可以的话,下一步想办法换成shell脚本,curl应该是也可以做到类似的请求功能的吧.只是组合出对应的逻辑可能比较麻烦
