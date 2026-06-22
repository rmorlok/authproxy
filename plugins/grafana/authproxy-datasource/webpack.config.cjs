const path = require('path');

const pluginId = 'rmorlok-authproxy-datasource';

module.exports = {
  context: path.resolve(__dirname, 'src'),
  devtool: 'source-map',
  entry: {
    module: './module.ts',
  },
  externals: [
    '@grafana/data',
    '@grafana/runtime',
    '@grafana/ui',
    'react',
    'react-dom',
    'react/jsx-runtime',
  ],
  module: {
    rules: [
      {
        exclude: /node_modules/,
        test: /\.[tj]sx?$/,
        use: {
          loader: 'swc-loader',
          options: {
            jsc: {
              parser: {
                syntax: 'typescript',
                tsx: true,
              },
              target: 'es2018',
              transform: {
                react: {
                  runtime: 'automatic',
                },
              },
            },
          },
        },
      },
    ],
  },
  optimization: {
    minimize: true,
  },
  output: {
    clean: true,
    filename: '[name].js',
    library: {
      type: 'amd',
    },
    path: path.resolve(__dirname, 'dist'),
    publicPath: `public/plugins/${pluginId}/`,
    uniqueName: pluginId,
  },
  resolve: {
    extensions: ['.ts', '.tsx', '.js', '.jsx'],
  },
};
