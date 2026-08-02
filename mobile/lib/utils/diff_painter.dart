/// Diff 解析与着色工具。
///
/// 提供两种解析模式：
/// - [parseDiff]：单栏 diff，逐行分类（context/add/remove/hunk/header）。
/// - [parseDiffSideBySide]：双栏并排 diff，按 hunk 配对（context/removeOnly/addOnly/mixed/header）。
///
/// 颜色规则见 SPEC §15。本文件仅产出结构化数据，不直接渲染颜色。
library;

/// 单栏 diff 行类型。
enum DiffLineKind { context, add, remove, hunk, header }

/// 单栏 diff 行。
class DiffLine {
  final String text;
  final DiffLineKind kind;
  const DiffLine(this.text, this.kind);
}

/// 双栏 diff 行类型。
enum DiffRowKind { context, removeOnly, addOnly, mixed, header }

/// 双栏 diff 一行（左/右各一栏，可能为空）。
class DiffRow {
  final String? leftText;
  final String? rightText;
  final int? leftLineNo;
  final int? rightLineNo;
  final DiffRowKind kind;

  const DiffRow({
    this.leftText,
    this.rightText,
    this.leftLineNo,
    this.rightLineNo,
    required this.kind,
  });
}

/// hunk 头正则：`@@ -oldStart[,oldLen] +newStart[,newLen] @@`
final RegExp _hunkRe =
    RegExp(r'^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@');

/// 解析单栏 diff。
///
/// 判定顺序（前缀越长先判）：
/// 1. `+++` / `---` → header
/// 2. `@@` → hunk
/// 3. `+` → add
/// 4. `-` → remove
/// 5. 其它 → context（含空行、单空格行）
List<DiffLine> parseDiff(String diff) {
  if (diff.isEmpty) return const [];
  final lines = diff.split('\n');
  final result = <DiffLine>[];
  for (final line in lines) {
    if (line.startsWith('+++') || line.startsWith('---')) {
      result.add(DiffLine(line, DiffLineKind.header));
    } else if (line.startsWith('@@')) {
      result.add(DiffLine(line, DiffLineKind.hunk));
    } else if (line.startsWith('+')) {
      result.add(DiffLine(line, DiffLineKind.add));
    } else if (line.startsWith('-')) {
      result.add(DiffLine(line, DiffLineKind.remove));
    } else {
      result.add(DiffLine(line, DiffLineKind.context));
    }
  }
  return result;
}

/// 单栏文字颜色。
int colorFor(DiffLineKind kind) {
  switch (kind) {
    case DiffLineKind.add:
      return 0xFF2E7D32;
    case DiffLineKind.remove:
      return 0xFFC62828;
    case DiffLineKind.hunk:
      return 0xFF1565C0;
    case DiffLineKind.header:
      return 0xFF616161;
    case DiffLineKind.context:
      return 0xFF424242;
  }
}

/// 单栏背景色（透明用 0x00000000 表示）。
int backgroundFor(DiffLineKind kind) {
  switch (kind) {
    case DiffLineKind.add:
      return 0x1A2E7D32;
    case DiffLineKind.remove:
      return 0x1AC62828;
    case DiffLineKind.hunk:
    case DiffLineKind.header:
    case DiffLineKind.context:
      return 0x00000000;
  }
}

/// 解析双栏并排 diff。
///
/// 算法：
/// 1. 按 `\n` 拆行，空行跳过
/// 2. `_hunkRe` 匹配 hunk 头 → flush 缓冲，进入 hunk 模式，记录行号
/// 3. hunk 之前：`---`/`+++` 作为 header，其它忽略
/// 4. hunk 内按首字符分类（空格/-/+/其它），缓冲 removes/adds
/// 5. flush 时按下标 zip 配对：mixed / removeOnly / addOnly
/// 6. 末尾再 flush 一次
List<DiffRow> parseDiffSideBySide(String diff) {
  if (diff.isEmpty) return const [];

  final lines = diff.split('\n');
  final rows = <DiffRow>[];

  // 缓冲（hunk 内的连续 -/+ 行配对）
  final removes = <_SideLine>[];
  final adds = <_SideLine>[];

  int? oldNo;
  int? newNo;
  bool inHunk = false;

  void flush() {
    if (removes.isEmpty && adds.isEmpty) return;
    final n = removes.length > adds.length ? removes.length : adds.length;
    for (var i = 0; i < n; i++) {
      final r = i < removes.length ? removes[i] : null;
      final a = i < adds.length ? adds[i] : null;
      final DiffRowKind kind;
      if (r != null && a != null) {
        kind = DiffRowKind.mixed;
      } else if (r != null) {
        kind = DiffRowKind.removeOnly;
      } else {
        kind = DiffRowKind.addOnly;
      }
      rows.add(DiffRow(
        leftText: r?.text,
        rightText: a?.text,
        leftLineNo: r?.lineNo,
        rightLineNo: a?.lineNo,
        kind: kind,
      ));
    }
    removes.clear();
    adds.clear();
  }

  for (final raw in lines) {
    if (raw.isEmpty) continue; // 末尾换行产生的伪行
    final m = _hunkRe.firstMatch(raw);
    if (m != null) {
      flush();
      inHunk = true;
      oldNo = int.tryParse(m.group(1) ?? '');
      newNo = int.tryParse(m.group(3) ?? '');
      rows.add(DiffRow(
        leftText: raw,
        rightText: raw,
        kind: DiffRowKind.header,
      ));
      continue;
    }

    if (!inHunk) {
      // hunk 之前：仅 ---/+++ 作为 header，其它忽略
      if (raw.startsWith('---') || raw.startsWith('+++')) {
        rows.add(DiffRow(
          leftText: raw,
          rightText: raw,
          kind: DiffRowKind.header,
        ));
      }
      continue;
    }

    // hunk 内
    if (raw.startsWith(r'\')) {
      // `\ No newline at end of file` 跳过
      continue;
    }
    if (raw.startsWith(' ')) {
      flush();
      final text = raw.substring(1);
      rows.add(DiffRow(
        leftText: text,
        rightText: text,
        leftLineNo: oldNo,
        rightLineNo: newNo,
        kind: DiffRowKind.context,
      ));
      if (oldNo != null) oldNo = oldNo + 1;
      if (newNo != null) newNo = newNo + 1;
    } else if (raw.startsWith('-')) {
      removes.add(_SideLine(text: raw.substring(1), lineNo: oldNo));
      if (oldNo != null) oldNo = oldNo + 1;
    } else if (raw.startsWith('+')) {
      adds.add(_SideLine(text: raw.substring(1), lineNo: newNo));
      if (newNo != null) newNo = newNo + 1;
    } else {
      // 罕见无前缀行：当 context 处理
      flush();
      rows.add(DiffRow(
        leftText: raw,
        rightText: raw,
        leftLineNo: oldNo,
        rightLineNo: newNo,
        kind: DiffRowKind.context,
      ));
      if (oldNo != null) oldNo = oldNo + 1;
      if (newNo != null) newNo = newNo + 1;
    }
  }
  flush();

  return rows;
}

/// 双栏左侧背景色。
int sideBackgroundLeft(DiffRowKind kind) {
  switch (kind) {
    case DiffRowKind.removeOnly:
    case DiffRowKind.mixed:
      return 0x1AC62828;
    case DiffRowKind.context:
      return 0x0D000000;
    case DiffRowKind.addOnly:
    case DiffRowKind.header:
      return 0x00000000;
  }
}

/// 双栏右侧背景色。
int sideBackgroundRight(DiffRowKind kind) {
  switch (kind) {
    case DiffRowKind.addOnly:
    case DiffRowKind.mixed:
      return 0x1A2E7D32;
    case DiffRowKind.context:
      return 0x0D000000;
    case DiffRowKind.removeOnly:
    case DiffRowKind.header:
      return 0x00000000;
  }
}

class _SideLine {
  final String text;
  final int? lineNo;
  const _SideLine({required this.text, required this.lineNo});
}
