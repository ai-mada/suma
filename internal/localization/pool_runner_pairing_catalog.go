package localization

var poolRunnerPairingCatalog = []Definition{
	{Key: "pool.runner_pairing.invalid", Texts: map[Locale]string{ZhCN: "资源池节点配对请求或分页参数无效", EnUS: "The pool runner pairing request or pagination parameters are invalid."}},
	{Key: "pool.runner_pairing.changed", Texts: map[Locale]string{ZhCN: "资源池节点配对依据已变化，请核对原请求", EnUS: "The pool runner pairing basis has changed. Review the original request."}},
	{Key: "pool.runner_pairing.expired", Texts: map[Locale]string{ZhCN: "资源池节点配对已过期，请创建新请求", EnUS: "The pool runner pairing has expired. Create a new request."}},
	{Key: "pool.runner_pairing.rejected", Texts: map[Locale]string{ZhCN: "资源池节点配对已被拒绝", EnUS: "The pool runner pairing was rejected."}},
	{Key: "pool.runner_pairing.limit", Texts: map[Locale]string{ZhCN: "此资源池已有八个等待中的节点配对", EnUS: "This pool already has eight active runner pairings."}},
	{Key: "pool.runner_pairing.unavailable", Texts: map[Locale]string{ZhCN: "资源池节点配对需要安全的控制面地址", EnUS: "Pool runner pairing requires a safe operator-controlled control URL."}},
}
