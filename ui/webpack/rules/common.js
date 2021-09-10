/**
 * Created by: Andrey Polyakov (andrey@polyakov.im)
 */
import {babelLoader} from './useLoaderRuleItems';
import path from "path"

/**
 * @see https://webpack.js.org/guides/typescript/#loader
 * @see https://github.com/thien-do/typed.tw/tree/master/webpack-loader
 */
export const typescriptRule = {
    test: /\.tsx?$/,
    exclude: /node_modules/,
    use: [
        {
            loader: "ts-loader",
            options: {transpileOnly: true}
        },
        {
            loader: "typed-tailwind-loader",
            options: { config: path.resolve("./tw.ts") }
        }
    ],
};

/**
 * @see https://webpack.js.org/loaders/babel-loader
 */
export const javascriptRule = {
    test: /\.(js|jsx)$/,
    use: [babelLoader],
    exclude: /node_modules/,
    include: /src/
};

/**
 * @see https://github.com/gaearon/react-hot-loader/issues/1227#issuecomment-482518698
 */
export const hmrRule = {
    test: /\.(js|jsx)$/,
    use: 'react-hot-loader/webpack',
    include: /node_modules/
};


/**
 * @see https://webpack.js.org/loaders/html-loader
 */
export const htmlRule = {
    test: /\.(html)$/,
    use: {
        loader: 'html-loader',
    },
};
/**
 * @see https://webpack.js.org/guides/asset-modules/
 */
export const imagesRule = {
    test: /\.(?:ico|gif|png|jpg|jpeg)$/i,
    type: 'asset/resource',
};
/**
 * @see https://webpack.js.org/guides/asset-modules/
 */
export const fontsRule = {
    test: /\.(woff(2)?|eot|ttf|otf|)$/,
    type: 'asset/inline',
};
