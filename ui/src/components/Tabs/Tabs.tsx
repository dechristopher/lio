import React, {Children, FC, ReactNode, useState} from 'react';
import classNames from "classnames";
import {Tab, TabProps} from "@components/Tabs/Tab";

interface TabsProps {
    children: ReactNode | ReactNode[];
}

interface ITabProps extends FC<TabsProps> {
    Tab: typeof Tab;
}

/**
 * A collection of tabs where each tab maintains it's own content.
 *
 * @param {ITabProps} props - Component props.
 *
 * @returns {Element} - Tabs component.
 *
 * @example
 * <Tabs />
 *  <Tabs.Tab title="example title" content="example content"/>
 *  <Tabs.Tab title="example title" content="example content"/>
 * </Tabs>
 */
const Tabs: ITabProps = (props) => {
    const tabTitles: ReactNode[] = [];
    const [selectedTab, setSelectedTab] = useState<number>(0);

    if (Array.isArray(props.children)) {
        Children.forEach(props.children, (child) => {
            if (React.isValidElement<TabProps>(child)) {
                tabTitles.push(child.props.title)
            }
        })
    } else {
        if (React.isValidElement<TabProps>(props.children)) {
            tabTitles.push(props.children.props.title)
        }
    }

    return (
        <div className="h-full flex flex-col">
            <div className="block">
                <div className="border-b border-gray-200">
                    <nav className="-mb-px flex justify-around" aria-label="Tabs">
                        {tabTitles.map((tabTitle, key) => {
                            return <a
                                key={key}
                                href="#"
                                onClick={() => setSelectedTab(key)}
                                className={
                                    classNames(
                                        "w-full",
                                        "py-4",
                                        "px-1",
                                        "text-center",
                                        "border-b-2",
                                        "font-medium",
                                        "text-sm",
                                        {
                                            "border-green-500 text-green-500": key === selectedTab,
                                            "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300": key !== selectedTab
                                        }
                                    )}>
                                {tabTitle}
                            </a>
                        })}
                    </nav>
                </div>
            </div>
            <div className="flex flex-1 justify-center items-center">
                {Children.map(props.children, (child, key) => {
                    return key === selectedTab ? child : null
                })}
            </div>
        </div>
    )
}

Tabs.propTypes = {
    children: (props, propName, componentName) => {
        const children = props[propName];

        let error = null;
        Children.forEach(children, (child) => {
            if (child.type !== Tab) {
                error = new Error(
                    `Direct children of ${componentName} must be of type 'Tabs.Tab'`
                )
            }
        })

        return error;
    }
}

Tabs.Tab = Tab;

export default Tabs;