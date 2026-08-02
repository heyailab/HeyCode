# 保留 Flutter 引擎相关类
-keep class io.flutter.app.** { *; }
-keep class io.flutter.plugin.**  { *; }
-keep class io.flutter.util.**  { *; }
-keep class io.flutter.view.**  { *; }
-keep class io.flutter.**  { *; }
-keep class io.flutter.plugins.**  { *; }

# 保留 Kotlin 元数据
-keep class kotlin.Metadata { *; }

# 保留 Parcelable 序列化对象
-keepclassmembers class * implements android.os.Parcelable {
    public static final android.os.Parcelable$Creator CREATOR;
}

# Play Core SplitCompat（Flutter 引擎引用但运行时不一定加载，忽略缺失类警告）
# 修复 R8: Missing class com.google.android.play.core.*
-dontwarn com.google.android.play.core.**
-dontwarn com.google.android.play.**

# 网络库（OkHttp / Dart VM HTTP）
-dontwarn okhttp3.**
-dontwarn okio.**

# Dart HTTP 网络栈引用的 JVM 类
-dontwarn javax.annotation.**
