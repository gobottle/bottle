'use babel';

// This file is Copyright 2015 Joe Fitzgerald.  All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Modifications by Kevin Stenerson for Reflexion Health Inc. Copyright 2016

export default {
  matchFunction: /^(?:func[(]{1})([^\)]*)(?:[)]{1})(?:$|(?:\s)([^\(]*$)|(?: [(]{1})([^\)]*)(?:[)]{1}))/i,

  mapMessages(data, editor, position) {
    if (!data) { return []; }

    let json;
    try {
      json = JSON.parse(data);
    } catch (e) {
      if (e && e.handle) { e.handle(); }
      atom.notifications.addError('gocode error', {detail: data, dismissable: true});
      console.log(e);
      return [];
    }

    const prefixSize = json[0];
    const candidates = json[1];
    if (!candidates) { return []; }

    let prefix;
    try {
      prefix = editor.getTextInBufferRange([[position.row, position.column - prefixSize], position]);
    } catch (e) {
      console.log(e)
      return [];
    }

    let suffix = false;
    try {
      suffix = editor.getTextInBufferRange([position, [position.row, position.column + 1]]);
    } catch (e) {
      console.log(e);
    }

    const suggestions = [];
    for (const c of candidates) {
      let suggestion = {
        replacementPrefix: prefix,
        leftLabel: c.type || c.class,
        type: this.translateType(c.class)
      };

      if (c.class === 'func' && (!suffix || suffix !== '(')) {
        suggestion = this.upgradeSuggestion(suggestion, c);
      } else {
        suggestion.text = c.name;
      }

      if (suggestion.type === 'package') {
        suggestion.iconHTML = '<i class="icon-package"></i>';
      }

      suggestions.push(suggestion);
    }

    return suggestions;
  },

  translateType(type) {
    if (type === 'func') {
      return 'function';
    }
    if (type === 'var') {
      return 'variable';
    }
    if (type === 'const') {
      return 'constant';
    }
    if (type === 'PANIC') {
      return 'panic';
    }
    return type;
  },

  upgradeSuggestion(suggestion, candidate) {
    if (!candidate || !candidate.type || candidate.type === '') {
      return suggestion;
    }

    const match = this.matchFunction.exec(candidate.type);
    if (!match || !match[0]) { // Not a function
      suggestion.snippet = candidate.name + '()';
      suggestion.leftLabel = '';
      return suggestion;
    }
    suggestion.leftLabel = match[2] || match[3] || '';

    const res = this.generateSnippet(candidate.name, match);
    suggestion.snippet = res.snippet;
    suggestion.displayText = res.displayText;
    return suggestion;
  },

  generateSnippet(name, match) {
    const signature = {snippet: name, displayText: name};
    if (!match || !match[1] || match[1] === '') { // no arguments
      return {snippet: name + '()$0', displayText: name + '()'};
    }

    const args = match[1].split(/, /).map((a) => {
      if (!a || a.length <= 2) {
        return {display: a, snippet: a};
      }
      if (a.substring(a.length - 2, a.length) === '{}') {
        return {display: a, snippet: a.substring(0, a.length - 1) + '\\}'};
      }
      return {display: a, snippet: a};
    });

    let i = 1;
    for (const arg of args) {
      if (this.snippetMode === 'name') {
        const parts = arg.snippet.split(' ');
        arg.snippet = parts[0];
      }
      if (i === 1) {
        signature.snippet = name + '(${' + i + ':' + arg.snippet + '}';
        signature.displayText = name + '(' + arg.display;
      } else {
        signature.snippet = signature.snippet + ', ${' + i + ':' + arg.snippet + '}';
        signature.displayText = signature.displayText + ', ' + arg.display;
      }
      i += 1;
    }

    signature.snippet = signature.snippet + ')$0';
    signature.displayText = signature.displayText + ')';

    if (this.snippetMode === 'none') {
      // user doesn't care about arg names/types
      signature.snippet = name + '($0)';
    }

    return signature;
  }
};
