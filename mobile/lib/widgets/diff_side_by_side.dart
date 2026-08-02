/// 双栏并排 diff 展示组件。
///
/// 根据 SPEC §14.11 实现：左红右绿配对，行号 + 内容，等宽字体。
/// 由 [SnapshotCard._showDiff] 通过 `showModalBottomSheet` 触发。
library;

import 'package:flutter/material.dart';

import '../utils/diff_painter.dart';

class DiffSideBySide extends StatelessWidget {
  final String diff;

  const DiffSideBySide({super.key, required this.diff});

  // 常量（SPEC §14.11）
  static const double _rowH = 20.0;
  static const double _lineNoW = 40.0;
  static const double _charW = 7.2;
  static const double _gap = 6.0;
  static const double _hPadding = 4.0;
  static const int _minChars = 40;

  static TextStyle get _mono => const TextStyle(
        fontFamily: 'monospace',
        fontFamilyFallback: ['RobotoMono', 'Courier'],
        fontSize: 12,
        height: 1.4,
      );

  @override
  Widget build(BuildContext context) {
    final rows = parseDiffSideBySide(diff);
    if (rows.isEmpty) {
      return Center(
        child: Text(
          '无 diff 内容',
          style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.outline,
              ),
        ),
      );
    }

    // 计算 maxChars（取所有左右文本最大长度，至少 _minChars）
    var maxChars = _minChars;
    for (final r in rows) {
      final lc = r.leftText?.length ?? 0;
      final rc = r.rightText?.length ?? 0;
      if (lc > maxChars) maxChars = lc;
      if (rc > maxChars) maxChars = rc;
    }

    return LayoutBuilder(
      builder: (context, constraints) {
        // 单侧宽度 = 行号列 + gap + 内容列
        final sideWidth = _lineNoW + _gap + _hPadding * 2 + maxChars * _charW;
        final rowWidth = sideWidth * 2;
        final viewportWidth = constraints.maxWidth.isFinite
            ? constraints.maxWidth
            : 800.0;
        final totalWidth =
            rowWidth > viewportWidth ? rowWidth : viewportWidth;

        return SingleChildScrollView(
          child: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: SizedBox(
              width: totalWidth,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final r in rows) _buildRow(context, r),
                ],
              ),
            ),
          ),
        );
      },
    );
  }

  Widget _buildRow(BuildContext context, DiffRow row) {
    if (row.kind == DiffRowKind.header) {
      return Container(
        color: const Color(0xFFE3F2FD),
        padding: const EdgeInsets.symmetric(horizontal: 6),
        alignment: Alignment.centerLeft,
        child: Text(
          row.leftText ?? '',
          style: _mono.copyWith(color: const Color(0xFF1565C0)),
          softWrap: false,
          maxLines: 1,
          overflow: TextOverflow.clip,
        ),
      );
    }
    return SizedBox(
      height: _rowH,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Expanded(
            child: _side(
              text: row.leftText,
              lineNo: row.leftLineNo,
              bg: sideBackgroundLeft(row.kind),
            ),
          ),
          Expanded(
            child: _side(
              text: row.rightText,
              lineNo: row.rightLineNo,
              bg: sideBackgroundRight(row.kind),
            ),
          ),
        ],
      ),
    );
  }

  Widget _side({
    required String? text,
    required int? lineNo,
    required int bg,
  }) {
    return DecoratedBox(
      decoration: BoxDecoration(color: Color(bg)),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: _hPadding),
        child: Row(
          children: [
            SizedBox(
              width: _lineNoW,
              child: Text(
                lineNo == null ? '' : lineNo.toString(),
                textAlign: TextAlign.right,
                style: _mono.copyWith(
                  color: const Color(0xFF9E9E9E),
                  fontSize: 11,
                ),
                softWrap: false,
                maxLines: 1,
                overflow: TextOverflow.clip,
              ),
            ),
            const SizedBox(width: _gap),
            Expanded(
              child: Text(
                text ?? '',
                style: _mono,
                softWrap: false,
                maxLines: 1,
                overflow: TextOverflow.clip,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
