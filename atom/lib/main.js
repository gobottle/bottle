'use babel';

import {BufferedProcess} from 'atom';
import gocode from './gocode';
import pathutil from 'path';

function run(cmd, args = [], options = {}) {
  return new Promise((resolve, reject) => {
    let stderr = "";
    let stdout = "";

    const child = new BufferedProcess({
      command: cmd, args: args,
      stdout: (data) => { stdout += data; },
      stderr: (data) => { stderr += data; },
      exit: (code) => { (code == 0) ? resolve(stdout) : reject(stderr); }
    });

    if (options.input) { child.process.stdin.end(options.input); }

    return child;
  });
}

function which(context) {
  return run('bottle', ['--which', context.path]).then((output) => {
    const pkginfo = /^\[(.+)\] (.+)/.exec(output);
    if (pkginfo) { context.root = pkginfo[2] }
  });
}

// The functions of the gocode object are Copyright 2014 Manu Martinez-Almeida.  All rights reserved.
// Use of this section of the source code is governed by a MIT style license.
//
// Modifications by Kevin Stenerson for Reflexion Health Inc. Copyright 2016


let gocodePanicked = false;
function autocomplete({editor, position, offset, input, root, path}) {
  if (atom.config.get('gobottle.gocode') && !gocodePanicked) {
    return run('which', ['gocode'])
          .then(() => {
                  const relpath = pathutil.relative(root, path);
                  const tool = 'gocode -f=json autocomplete ' + relpath + ' ' + offset;
                  return run('bottle', ['--tool', tool, root], {input})
                        .then((output) => {
                          const suggest = gocode.mapMessages(output, editor, position)
                          if (suggest[0] && suggest[0].type == "panic") {
                            atom.notifications.addWarning("Disabled gobottle autocomplete provider", {
                              dismissable: true,
                              detail: "The provider was disabled because the \"gocode\" tool crashed.\n"
                                      + "This could be a bug in https://github.com/nsf/gocode.\n"
                                      + "\t\n"
                                      + "Go autocompletion will be disabled until atom restarts, to\n"
                                      + "allow autocomplete-plus to continue to provide its file-\n"
                                      + "based default suggestions."
                            });

                            gocodePanicked = true;
                            return undefined;
                          }
                          return suggest;
                        })
                        .catch(() => {});
                },
                () => console.log('gobottle: could not find gocode'))
          .catch(() => console.log('gobottle: error running gocode'));
  }
  return Promise.resolve();
}

function format({editor, root, path}, linter) {
  if (!editor.isModified()) {
    if (atom.config.get('gobottle.goimports')) {
      return run('which', ['goimports'])
            .then(() => {
                    const relpath = pathutil.relative(root, path);
                    const tool = 'goimports -w ' + relpath;
                    return run('bottle', ['--tool', tool, root])
                          // .then((output) => atom.notifications.addSuccess("Imports updated"))
                          .catch(() => {});
                  },
                  () => console.log('gobottle: could not find goimports'))
            .catch(() => console.log('gobottle: error running goimports'));
    }

    // TODO: maybe support `go fmt` as a fallback (or default?) option

  }
  return Promise.resolve();
}

function lint(context, linter) {
  const {editor, root, path} = context;
  const regexp = '(?<file>[A-Za-z0-9\\-_.][A-Za-z0-9\\-_./ ]*\.go):(?<line>\\d+):((?<col>\\d+):)? (?<message>(.*(\n\t)?)*)';
  const promise = linter.exec('bottle', [path], {stream: 'both', allowEmptyStderr: true});
  return promise.then(({stdout, stderr}) => {
    // figure out where we are for project-relative file paths
    const location = atom.project.getPaths()
      .map((dir) => pathutil.relative(dir, root))
      .reduce((min, curr) => (curr < min) ? curr : min);

    // parse the errors from the compiler
    let messages = linter.parse(stderr, regexp).map((message) => {
      message.type = 'Error';
      message.filePath = pathutil.join(location, message.filePath);
      return message;
    });

    // if we have error output but 0 errors, use all of stderr as the message
    if ((!messages || messages.length == 0) && stderr.length > 0) {
      messages = [{type: 'Error', text: stderr}];
    }

    context.messages = messages;
    return messages;
  });
}

export default {
  config: {
    gocode: {
      title: 'Use gocode for autocomplete',
      description: 'Use gocode for autocomplete suggestions with autocomplete-plus',
      type: 'boolean',
      default: true
    },
    goimports: {
      title: 'Run goimports on save',
      description: 'Enable this to run goimports on .go files when you save them',
      type: 'boolean',
      default: true
    }
  },
  activate() { require('atom-package-deps').install('gobottle'); },
  provideAutocomplete() {
    return {
      selector: '.source.go',
      inclusionPriority: 1,
      excludeLowerPriority: true,
      getSuggestions({editor, bufferPosition}) {
        const context = {root: '', path: editor.getPath()}
        const buffer = editor.getBuffer();
        const index = buffer.characterIndexForPosition(bufferPosition);
        const text = editor.getText();
        if (index > 0 && ', \t\n'.indexOf(text[index - 1]) > -1) { return; }

        context.input = text;
        context.editor = editor;
        context.offset = Buffer.byteLength(text.substring(0, index), 'utf8');
        context.position = bufferPosition;
        return which(context).then(() => autocomplete(context));
        // return Promise.resolve([]);
      }
    }
  },
  provideLinter() {
    const linter = require('atom-linter');
    return {
      name: 'gobottle',
      grammarScopes: ['source.go'],
      scope: 'project',
      lintOnFly: false,
      lint(editor) {
        const context = {root: '', path: editor.getPath(), editor: editor, messages: []};
        return which(context)
              .then(() => format(context, linter))
              .then(() => lint(context, linter));
      }
    };
  }
}
