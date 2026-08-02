import 'package:flutter/material.dart';

/// 加载指示器，居中显示 CircularProgressIndicator + 可选 label。
class LoadingIndicator extends StatelessWidget {
  final String? label;

  const LoadingIndicator({super.key, this.label});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const CircularProgressIndicator(),
          if (label != null) ...[
            const SizedBox(height: 12),
            Text(label!, style: Theme.of(context).textTheme.bodyMedium),
          ],
        ],
      ),
    );
  }
}
