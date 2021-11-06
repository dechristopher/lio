import React, {CSSProperties, FC} from "react";
import classNames from "classnames";

interface IContentContainerProps {
    className?: string;
    style?: CSSProperties;
}

export const ContentContainer: FC<IContentContainerProps> = ( props ): JSX.Element => {
    return (
        <div
            style={props.style}
            className={classNames(
                "container",
                props.className
            )}
        >
            {props.children}
        </div>
    )
}