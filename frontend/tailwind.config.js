/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: [
          '-apple-system',
          'BlinkMacSystemFont',
          '"SF Pro Text"',
          '"Segoe UI"',
          'Roboto',
          'Helvetica',
          'Arial',
          'sans-serif',
        ],
      },
      colors: {
        ios: {
          bg: '#F2F2F7', 
          bgDark: '#000000',
          card: '#FFFFFF',
          cardDark: '#1C1C1E',
          blue: '#007AFF',
          blueDark: '#0A84FF',
          green: '#34C759',
          greenDark: '#30D158',
          red: '#FF3B30',
          redDark: '#FF453A',
          text: '#000000',
          textDark: '#FFFFFF',
          textSecondary: 'rgba(60, 60, 67, 0.6)',
          textSecondaryDark: 'rgba(235, 235, 245, 0.6)',
          divider: 'rgba(60, 60, 67, 0.29)',
          dividerDark: 'rgba(84, 84, 88, 0.65)'
        }
      },
      // iOS 设计 token：所有面板 / 弹层 / 圆 chip 圆角全部走这里。
      // 抽 token 是为了避免「同一种角，散布在 30+ 处用 rounded-[16px]/[28px]/full」的不一致。
      borderRadius: {
        'ios-card': '28px',     // 大面板 / 抽屉 / modal 主体
        'ios-block': '16px',    // 内嵌区块 / 列表项 / banner / chip group
        'ios-tile': '12px',     // 小图标盒 / 工具按钮
        'ios-pill': '9999px',   // 单 chip / pill 按钮
      },
      // iOS 卡片 / 弹层标准阴影。给关键面板用，避免各 view 自写
      // shadow-[0_-20px_60px_rgba(...)] 这种长串。
      boxShadow: {
        'ios-card': '0 1px 2px rgba(0,0,0,0.04), 0 8px 24px rgba(0,0,0,0.06)',
        'ios-sheet': '0 -20px 60px rgba(0,0,0,0.30)',
      }
    },
  },
  plugins: [],
}
