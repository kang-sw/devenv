import hashlib, json, os, pathlib, shlex, subprocess, sys, time
root = pathlib.Path('/tmp/ws-board-topics-common-history.EWq6tq')
repo = pathlib.Path('/Users/kang-sw/.ws-worktrees/devenv/dense-hardship-outclass')
revisions = {'baseline': 'b84a0de4879168f4c4c0b1bfbd74ceeaad7d7bb7', 'candidate': '2b3e54f9ae745bc77f3fdd9840136306593e37ad'}
def git(*args):
    return subprocess.check_output(['git', *args], cwd=repo)
def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()
def inventory(path):
    return {str(p.relative_to(path)): sha(p) for p in sorted(path.rglob('*')) if p.is_file() and not p.is_symlink()}
def save():
    (root/'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
    lines = ['# Common-history build/test evidence', '', 'No source edits or commits. Original LOOK failure remains open.', '', '## Identities']
    for name, item in manifest['sources'].items():
        lines += [f'- {name}: {item["revision"]}', f'  - Git module tree: {item["module_git_tree"]}', f'  - Export file inventory SHA256: {item["inventory_sha256"]}', f'  - Archive SHA256: {item["archive_sha256"]}', f'  - stream.js SHA256: {item["stream_sha256"]}']
        if 'binary_sha256' in item: lines += [f'  - Binary SHA256: {item["binary_sha256"]}']
    lines += ['', '## Commands', '', 'All commands use the sanitized environment recorded in manifest.json. The inherited Go module cache is read-only dependency input; no dependencies are installed.']
    for item in manifest['commands']:
        lines += ['', f'### {item["name"]}', f'Working directory: `{item["cwd"]}`', f'Command: `{shlex.join(item["argv"])}`', f'Exit: {item["exit"]}; elapsed: {item["elapsed_seconds"]} seconds.', f'Full output: `logs/{item["name"]}.log`', '```text', (root/'logs'/f'{item["name"]}.log').read_text(), '```']
    (root/'README.md').write_text('\n'.join(lines)+'\n')
manifest = {'scratch_root':str(root), 'head':git('rev-parse','HEAD').decode().strip(), 'branch':git('branch','--show-current').decode().strip(), 'sources':{}, 'commands':[], 'required_diff_empty':not git('diff',revisions['baseline'],revisions['candidate'],'--','agents-plugin-tool/internal/wswhiteboard','scripts/board-perf.mjs','scripts/board-headless.mjs'), 'old_look_failure':'preserved and open'}
assert manifest['head'] == revisions['candidate'] and manifest['required_diff_empty']
for folder in ('bin','logs','exports','state'):
    (root/folder).mkdir(exist_ok=True)
for name, revision in revisions.items():
    archive=root/f'{name}.tar'
    with archive.open('wb') as out:
        subprocess.run(['git','archive','--format=tar',revision],cwd=repo,stdout=out,check=True)
    dest=root/'exports'/name
    dest.mkdir()
    subprocess.run(['tar','-xf',str(archive),'-C',str(dest)],check=True)
    files=inventory(dest)
    encoded=json.dumps(files,sort_keys=True,indent=2)+'\n'
    (root/f'{name}-files.sha256.json').write_text(encoded)
    manifest['sources'][name]={'revision':revision,'root':str(dest),'module_git_tree':git('rev-parse',revision+':agents-plugin-tool').decode().strip(),'root_git_tree':git('rev-parse',revision+'^{tree}').decode().strip(),'archive_sha256':sha(archive),'inventory_sha256':hashlib.sha256(encoded.encode()).hexdigest(),'stream_sha256':sha(dest/'agents-plugin-tool/internal/wswhiteboard/assets/stream.js')}
    assert manifest['sources'][name]['stream_sha256']=='5a7e86ab9dc8624e2296335d113d69dcd349aaa65c0e0cbb8765aa806634555a'
    for p in dest.rglob('*'):
        if not p.is_symlink(): p.chmod(p.stat().st_mode & ~0o222)
    dest.chmod(dest.stat().st_mode & ~0o222)
    archive.chmod(0o444)
env={k:v for k,v in os.environ.items() if not k.startswith('WS_') and k not in ('GOCACHE','GOTOOLCHAIN','GOPROXY','GOFLAGS','GOWORK','GOMODCACHE')}
env.update(GOTOOLCHAIN='local',GOPROXY='off',GOFLAGS='-mod=readonly',GOWORK='off')
modcache=subprocess.check_output(['go','env','GOMODCACHE'],env=env,text=True).strip()
env['GOMODCACHE']=modcache
manifest['environment']={'removed_inherited_names': sorted(k for k in os.environ if k.startswith('WS_')), 'common_overrides':{k:env[k] for k in ('GOTOOLCHAIN','GOPROXY','GOFLAGS','GOWORK','GOMODCACHE')}, 'go_version':subprocess.check_output(['go','version'],env=env,text=True).strip()}
save()
def run(name, side, args):
    state=root/'state'/side
    state.mkdir(exist_ok=True)
    command_env=env.copy()
    for key, folder in [('WS_CACHE_HOME','ws-cache'),('WS_CONFIG_HOME','ws-config'),('GOCACHE','go-cache')]:
        path=state/folder; path.mkdir(exist_ok=True); command_env[key]=str(path)
    cwd=root/'exports'/side/'agents-plugin-tool'
    start=time.time()
    with (root/'logs'/f'{name}.log').open('wb') as log:
        result=subprocess.run(args,cwd=cwd,env=command_env,stdout=log,stderr=subprocess.STDOUT)
    record={'name':name,'side':side,'cwd':str(cwd),'argv':args,'environment':{k:command_env[k] for k in ('WS_CACHE_HOME','WS_CONFIG_HOME','GOCACHE')},'exit':result.returncode,'elapsed_seconds':round(time.time()-start,3)}
    manifest['commands'].append(record)
    manifest['sources'][side]['export_unchanged']=inventory(root/'exports'/side)==json.loads((root/f'{side}-files.sha256.json').read_text())
    binary=root/'bin'/f'{side}.test'
    if binary.exists(): manifest['sources'][side]['binary_sha256']=sha(binary)
    save()
    print(json.dumps(record),flush=True)
    if result.returncode or not manifest['sources'][side]['export_unchanged']:
        print('STOP: command failed or source export changed; no rerun or edits.',flush=True);sys.exit(1)
for side in revisions:
    run(side+'-build',side,['go','test','-c','-count=1','-o',str(root/'bin'/f'{side}.test'),'./internal/mcp'])
for side in revisions:
    run(side+'-suites',side,['go','test','-timeout','20m','./internal/wsdashboard','./internal/wswhiteboard','-count=1'])
run('candidate-mcp-subset','candidate',['go','test','-timeout','20m','./internal/mcp','-run','Dashboard|Whiteboard|Board|Panel|Theme|Closed|Topic|Wiki','-count=1'])
run('candidate-focused','candidate',['go','test','-timeout','20m','./internal/wsdashboard','-run','TestSplitBoardTopics|TestBoardTopics|TestPageStreamsRideWebSocket','-count=1'])
manifest['final_worktree_status']=git('status','--porcelain').decode()
manifest['final_head']=git('rev-parse','HEAD').decode().strip()
save()
print('All requested commands finished; full outputs require reading.',flush=True)
