rebuild:
	rm -rf node_modules
	rm -rf dist
	npm cache clean --force
	npm install --prefer-offline --no-audit
	npm prune
	npm run build
