#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Flask Web 界面 for MCP 客户端
"""

import logging
import logging.config
from flask import Flask, render_template, request, jsonify
from client import auto_call_mcp, init_yml_cfg

# 配置日志
logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

# 创建 Flask 应用
app = Flask(__name__)

# 初始化配置
cfg = init_yml_cfg()


@app.route('/')
def index():
    """主页面"""
    return render_template('index.html')


@app.route('/api/query', methods=['POST'])
def process_query():
    """处理用户查询的 API 端点"""
    try:
        # 获取用户输入
        data = request.get_json()
        if not data or 'question' not in data:
            return jsonify({'error': '缺少问题参数'}), 400

        question = data['question']
        logger.info(f"收到用户查询: {question}")

        # 调用 MCP 客户端处理查询
        result = auto_call_mcp(question, cfg)

        # 返回结果
        return jsonify({
            'success': True,
            'question': question,
            'answer': result
        })

    except Exception as e:
        logger.exception("处理查询时发生错误")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


@app.route('/api/health')
def health_check():
    """健康检查端点"""
    return jsonify({'status': 'healthy'})


if __name__ == '__main__':
    # 启动 Flask 应用
    app.run(debug=True, host='0.0.0.0', port=19001)