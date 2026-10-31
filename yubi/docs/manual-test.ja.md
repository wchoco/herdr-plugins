# yubi 手動テストのチェックリスト

[manual-test.md](manual-test.md) の日本語版。内容を変えるときは両方を同時に直す。

`yubi/vX.Y.Z` のタグを打つ前に毎回通す。前提として、herdr と tmux が入った環境で `scripts/check.sh` が通っていること（中の `yubi/scripts/e2e.sh` が、再起動とズームの場合を使い捨ての herdr で自動で確かめている）。ここで確かめるのは、それらでは見られないもの：実際の端末、クリップボード、SSH、見た目。チェックリストはリリース PR に貼り、そこで各項目にチェックを入れる。各節で使った端末（とバージョン）を書き添える。

## 準備

1. タグを打つコミットをチェックアウトし、ビルドしてから link する。`plugin link` はマニフェストのビルドコマンドを実行しないので、先にビルドする。変更のたびにビルドし直す：

   ```sh
   herdr plugin unlink wchoco.yubi 2>/dev/null; herdr plugin uninstall wchoco.yubi 2>/dev/null
   (cd yubi && go build -o yubi .)
   herdr plugin link ./yubi
   ```

2. [README](../README.md#keybindings) のキーバインドを herdr の `config.toml` に足し、`herdr server reload-config` を実行する。
3. `$(herdr plugin config-dir wchoco.yubi)` にある `config.toml` を一時的に別の場所へ移し、既定値で動かす。終わったら戻す。
4. テスト画面のスクリプトを一度だけ作る：

   ```sh
   cat > /tmp/yubi-screen.sh <<'EOF'
   clear
   printf '%s\n' \
     'see https://example.com/docs and https://example.com/docs again' \
     'log written to /tmp/yubi/build.log' \
     'commit 1a2b3c4d5e6f7a8b' \
     'first line   ' \
     'second line'
   echo "https://example.com/$(printf 'w%.0s' $(seq 1 300))"
   EOF
   ```

   以下で「テスト画面」とは、zsh か bash のペインで `sh /tmp/yubi-screen.sh` を実行した直後の画面のこと。最後の行は、何行にもわたって折り返す長い URL。`first line` は末尾に空白 3 つ付きで出力される。プロンプト自体（`~/proj` など）に Match が含まれることもある。

「クリップボードを確かめる」とは、いま座っているマシンで、herdr の外のテキスト欄にデスクトップの貼り付けショートカットで貼り、期待するテキストと比べること。

## バージョン

- [ ] プラグインのルートで `./yubi version` が、マニフェストの `version`、`+`、短いコミット ID の順に出力し、`.dirty` が付かない。

## キー：kitty 対応の端末

kitty キーボードプロトコルに対応した端末（kitty、Ghostty、WezTerm、foot）で、herdr をローカルで動かし、テスト画面で行う。

- [ ] `prefix+f`：両方の URL（同じ Hint を共有）、`/tmp/yubi/build.log`、`1a2b3c4d5e6f7a8b` に Hint が出る。それ以外は薄く表示され、ステータス行に `HINT` と出る。
- [ ] `/tmp/yubi/build.log` の Hint を打つ：Enter なしですぐにオーバーレイが閉じ、herdr の「copied」トーストが出て、クリップボードに `/tmp/yubi/build.log` が入っている。
- [ ] `prefix+f` のあと、どの Hint の先頭でもない文字を打つ：何も起きない。2 文字の Hint の 1 文字目を打つ：ほかの Hint が消え、打った文字が薄くなる。Backspace で元に戻る。Esc で、目に見える遅れなくすぐにオーバーレイが閉じ、何もコピーされない。
- [ ] `prefix+y` で `second line` の Line Number を打って Enter：クリップボードに `second line` が入っている。
- [ ] `prefix+y` で `first line` と `second line` を選び（例 `N-M`）Enter：クリップボードに `first line` と `second line` が 2 行で入っている。末尾の空白も、最後の改行もない。
- [ ] `prefix+y` で折り返した URL の行をすべて選び **Ctrl-Enter**：クリップボードに URL が改行なしの 1 行で入っている。
- [ ] **Ctrl-J** でも同じ：同じ結果になる。
- [ ] `prefix+y` で `0` を打って Enter：ステータス行に `invalid input` と出て、オーバーレイは開いたまま。いちばん上の Line Number より大きい数を打って Enter：`no line N` と出る。Ctrl-U で入力が消える。Esc で閉じる。
- [ ] `prefix+y` のあと Esc：すぐにオーバーレイが閉じる。

## キー：古い端末

kitty キーボードプロトコルのない端末（xterm、GNOME Terminal、macOS の Terminal.app など）で、テスト画面で行う。

- [ ] `prefix+y` で折り返した URL の行を選び **Ctrl-J**：クリップボードに URL が 1 行で入っている。
- [ ] 同じ選択で **Ctrl-Enter**：何が起きたかを記録する。端末がただの Enter を送るので、通常のコピー（改行は残り、オーバーレイが閉じる）になるのが想定どおり。
- [ ] `prefix+y` で `second line` を選び **Alt-Enter**：シェルのプロンプトに `second line` が貼られ、実行はされない。
- [ ] `prefix+y` のあと Esc：少し遅れて（約 150 ms）オーバーレイが閉じる。何もコピーされない。
- [ ] `prefix+y` のあと Esc、すぐに `1`：オーバーレイが開いたままのことがある（Esc と `1` がまとめて Alt-1 になり、無視される）。そのあと Esc で閉じる。

## クリップボード

- [ ] **ローカル**、OSC 52：上の手順で確認済み。毎回「copied」トーストが出た。
- [ ] **tmux の中**：`set -g set-clipboard on` にした tmux の中で herdr を動かす。Hint Mode でコピーし、クリップボードを確かめる。
- [ ] **SSH 越し**：yubi を link した別のマシンに `ssh` し、そこで herdr を動かす。Hint Mode でコピーし、手元のマシンのクリップボードを確かめる。
- [ ] **`--remote`**：サーバーに yubi を install か link し、両方のマシンの herdr 設定にキーバインドを足す。`herdr --remote <ssh-target>` を実行し、Line Mode でコピーし、手元のマシンのクリップボードを確かめる。
- [ ] **`copy_command`**：yubi の `config.toml` に次を書く：

  ```toml
  [clipboard]
  copy_command = ["sh", "-c", "cat > /tmp/yubi-copy.txt"]
  ```

  Line をコピーすると、`cat /tmp/yubi-copy.txt` がちょうどその行を表示する。「copied」トーストは出ない。
- [ ] これを `["sh", "-c", "echo no display >&2; exit 4"]` に変える。コピーすると `copy failed: sh: exit status 4: no display` と出て、どのキーでもオーバーレイが閉じる。`config.toml` はまた取り除く。

## Paste Copy

bracketed paste を有効にする zsh か bash のプロンプトで、テスト画面で行う。

- [ ] `prefix+f` で `/tmp/yubi/build.log` の Hint を、1 文字目を**大文字**にして打つ：2 文字の Hint なら 1 文字目のあとにステータス行に `paste` の印が出る（1 文字の Hint ならすぐに貼られる）。パスはプロンプトに貼られ、実行されない。クリップボードにも入っている。
- [ ] `prefix+y` で `first line` と `second line` を選び **Alt-Enter**：2 行がひとつの貼り付けとしてシェルの入力バッファに入り、**実行されない**。Ctrl-U か Ctrl-C でバッファを消す。
- [ ] `prefix+y` で同じ選択をして **Alt-Ctrl-J**：`first line   second line` が 1 行で入り、実行されない（Joined Copy では 1 行目の末尾の空白が残る）。
- [ ] kitty 対応の端末で、同じことを **Alt-Ctrl-Enter** で行う：同じ結果になる。

## レイアウト

- [ ] **分割**：タブを 3 つに分ける（`herdr pane split --current --direction right`、続いて新しいペインで `herdr pane split --current --direction down`）。そのうち 1 つにテスト画面を出してフォーカスする。`prefix+f` と `prefix+y`：Target Pane がもとの位置にそのまま描き直される。ほかのペインはもとの位置に表示され、ペインの間の罫線は薄い。画面のどこも動かない。ほかのペインの Hint と Line Number は [Neighbour Pane](#neighbour-pane) で確かめる。
- [ ] **分割・狭い**：狭い分割ペインに、右端で切れる URL を出す。`prefix+f`：その Hint は見えている最後のセルに右寄せされ、打つと URL 全体がコピーされる。
- [ ] **ズーム中**：分割ペインをズームする（`herdr pane zoom --current --on`）。`prefix+f` と `prefix+y`：ズーム中のペインがもとの位置に全面の大きさで描き直され、ほかのペインも罫線も出ない。コピーもできる。オーバーレイが閉じたあとも、ペインはズームしたまま。バインドをもう一度押して閉じたとき（`prefix+f` を 2 回）と、`prefix+f`、`prefix+y`、Esc のあとも同じ。
- [ ] **タブ全体を占める 1 つのペイン**：ステータス行が最後の行を覆い、右端の何列かが切れる（README の既知の制限を参照）。コピーはできる。

## Neighbour Pane

上の**分割**と同じようにタブを 3 つに分ける：左のペイン、右上のペイン、右下のペイン。左のペインと右下のペインにテスト画面を出し、右上のペインで `ls -d /*` を実行してから、右下のペインにフォーカスする。右下が Target Pane で、左のペインが Neighbour Pane `n`、右上のペインが Neighbour Pane `b` になる（ラベルはアルファベットの末尾から、上から下、次に左から右の順に付く）。

- [ ] `prefix+f`：3 つのペインすべてに Hint が出る。左のペインの Hint は `n` で始まり（例：`na`）、右上のペインの Hint は `b` で始まり、ラベルも含めて描かれる。Target Pane の Hint は 1〜2 文字で、`n` や `b` で始まるものはない。
- [ ] テスト画面の URL には、Target Pane で Hint が 1 つ、左のペインで `n` の Hint が 1 つ付く。
- [ ] `n` を打つ：左のペインの Hint だけが残り、`n` は薄く描かれる。Backspace ですべての Hint が戻る。左のペインの Hint を最後まで打つ（例：`na`）：オーバーレイが閉じ、クリップボードに左のペインのその Match が入っている。
- [ ] `prefix+f` で `b` の Hint を打つ：クリップボードに右上のペインのそのパスが入っている。
- [ ] `prefix+f` で `n` の Hint を、1 文字目を大文字にして打つ（例：`Na`）：`N` のあとに `paste` の印が出て、テキストは Target Pane（右下のペイン）のプロンプトに貼られ、実行されない。左のペインには貼られない。
- [ ] 同じレイアウトで 3 回開く：ラベルも Hint も毎回同じ。
- [ ] `prefix+y`：すべてのペインに Line Number が出て、それぞれのペインの最後の空でない行から 1 で数え上がる。左のペインの Line Number はラベルを前に付けて描かれ（`n1`、`n2`、…）、右上のペインは `b` が付き、Target Pane にはラベルが付かない。
- [ ] `prefix+y` で `n` と、左のペインの `second line` の Line Number を打つ（例：`n3`）：その行が左のペインでだけ反転表示される。Enter：クリップボードに左のペインの `second line` が入っている。
- [ ] `prefix+y` で Target Pane の `second line` の Line Number をラベルなしで打って Enter：ペインが 1 つのときと同じく、クリップボードに Target Pane の `second line` が入っている。
- [ ] `prefix+y` で `3,n2` を打って Enter：ステータス行に `one pane at a time` と出て、オーバーレイは開いたまま。Ctrl-U のあと `x1` を打って Enter：`invalid input` と出る。Esc：何もコピーされていない。
- [ ] `prefix+y` で左のペインの `first line` と `second line` を選び（例：`n4-3`）、**Alt-Ctrl-J**：`first line   second line` が 1 行で Target Pane（右下のペイン）のプロンプトに貼られ、実行されない。左のペインには貼られない。
- [ ] `prefix+f` のあと `prefix+y`：どちらのモードでもラベルは同じ（左のペインが `n`、右上が `b`）。
- [ ] **短いアルファベット**：yubi の `config.toml` に `hint.alphabet = "asd"` を書く。`prefix+f`：左のペインにラベル `d` が付き、右上のペインには Hint が出ず薄く表示されるだけで、Target Pane の Hint は `a` と `s` だけを使う。`config.toml` はもとに戻す。
- [ ] **空の Target**：Target Pane（右下のペイン）で `clear` を実行する。`prefix+f`：`nothing to copy` は出ず、Hint Mode が開いてほかのペインに Hint が出る。`n` の Hint を打つ：クリップボードに左のペインのその Match が入っている。`prefix+y`：Line Mode が開く。`n1` を打って Enter：クリップボードに左のペインの一番下の行が入っている。
- [ ] **どこにも何もない**：3 つのペインすべてで `clear` を実行し、右下のペインにフォーカスする。`prefix+f`：`nothing to copy` が普通の文字で出る。左のペインにただの単語をいくつか出す：`prefix+f` で `no matches` と出る。どちらもどのキーでも閉じる。
- 読めない Neighbour Pane（空白で、ステータス行に `pane <label> unreadable`）は、本物の herdr ではわざと作れない。読み取りが失敗するのは、レイアウトを読んでからそのペインを読むまでのあいだにペインがなくなったときだけだからだ。これは自動テストとレンダリングのゴールデン `hint-split3-unreadable` で確かめている。

## 再起動とモード切り替え

- [ ] `prefix+f` のあと、もう一度 `prefix+f`：オーバーレイが閉じる。
- [ ] `prefix+y` のあと、もう一度 `prefix+y`：オーバーレイが閉じる。
- [ ] `prefix+f` のあと `prefix+y`：同じ Target Pane のまま Line Mode に切り替わる（ステータス行に `LINE`）。そこでコピーすると、もとのペインからコピーされる。
- [ ] これらのどれのあとでも、別のペインで `prefix+f` を押すと、そのペインで yubi が開く。

## 自動で閉じる場合

- [ ] **Target が閉じる**：Target Pane で `sleep 5; exit` を実行し、5 秒以内に `prefix+y` を押す。シェルが終了すると、オーバーレイがメッセージなしで自動で閉じる。
- [ ] **リサイズ**：オーバーレイを開いて端末のウィンドウの大きさを変える：オーバーレイがメッセージなしで自動で閉じる。
- [ ] **誤ったリサイズがない**：オーバーレイを開いて数秒待ち、同じバインドをもう一度押す：オーバーレイが閉じる（自動で閉じてはおらず、`cannot read target` のメッセージも出ない）。
- [ ] **フォーカスが離れる**：オーバーレイを開き、herdr のキーで別のタブに移る：戻ってきたときにはオーバーレイが消えている。
- [ ] **ズーム解除**：分割したタブでオーバーレイを開き、herdr のズーム切り替えキーを使う：オーバーレイが自動で閉じる。
- [ ] **Neighbour が閉じる**：[Neighbour Pane](#neighbour-pane) の 3 分割のレイアウトで、右上のペインで `sleep 5; exit` を実行し、左のペインにフォーカスして 5 秒以内に `prefix+f` を押す。シェルが終了すると、オーバーレイが自動で閉じる。これは yubi ではなく、ペインのどれかが閉じるか終了したときに herdr がタブのズームを解除するためだ。yubi 自身は Neighbour Pane が閉じても、終了しても、移動してもオーバーレイを開いたままにする（自動テストで確かめている）が、herdr にはそれが起こるレイアウトがない。herdr はズームしたタブからペインを移動させないので、オーバーレイが開いているあいだに Neighbour Pane が移動することもない。

## メッセージと設定エラー

- [ ] 空の画面（`clear`）で `prefix+f` を押すと `nothing to copy` と出る。どのキーでも閉じる。
- [ ] ただの単語しかない画面で `prefix+f` を押すと `no matches` と出る。どのキーでも閉じる。
- [ ] yubi の `config.toml` に `[hint]` と `alphabet = "aa"` を書く。`prefix+f` を押すと `config: hint.alphabet: "a" repeats` と出て、何も選べず、どのキーでも閉じる。ファイルを消すと、リロードしなくても次の起動から元どおり動く。
