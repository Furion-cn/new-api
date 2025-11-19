import React, { useEffect, useState } from 'react';
import {
  API,
  copy,
  showError,
  showSuccess,
} from '../helpers';

import { ITEMS_PER_PAGE } from '../constants';
import {
  Button,
  Divider,
  Space,
  Table,
  Tag,
} from '@douyinfe/semi-ui';

import { useTranslation } from 'react-i18next';

const BatchJobTable = () => {
  const { t } = useTranslation();

  const renderState = (state) => {
    switch (state) {
      case 'JOB_STATE_PENDING':
        return (
          <Tag color='blue' size='large'>
            {t('待处理')}
          </Tag>
        );
      case 'JOB_STATE_RUNNING':
        return (
          <Tag color='green' size='large'>
            {t('运行中')}
          </Tag>
        );
      case 'JOB_STATE_SUCCEEDED':
        return (
          <Tag color='green' size='large'>
            {t('成功')}
          </Tag>
        );
      case 'JOB_STATE_FAILED':
        return (
          <Tag color='red' size='large'>
            {t('失败')}
          </Tag>
        );
      case 'JOB_STATE_CANCELLED':
        return (
          <Tag color='grey' size='large'>
            {t('已取消')}
          </Tag>
        );
      default:
        return (
          <Tag color='black' size='large'>
            {state || t('未知状态')}
          </Tag>
        );
    }
  };

  const formatDateTime = (dateTimeStr) => {
    if (!dateTimeStr) return '-';
    try {
      const date = new Date(dateTimeStr);
      return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      });
    } catch (e) {
      return dateTimeStr;
    }
  };

  const columns = [
    {
      title: t('ID'),
      dataIndex: 'ID',
      width: 80,
    },
    {
      title: t('显示名称'),
      dataIndex: 'display_name',
      width: 150,
    },
    {
      title: t('状态'),
      dataIndex: 'state',
      width: 120,
      render: (text, record) => {
        return renderState(text);
      },
    },
    {
      title: t('模型'),
      dataIndex: 'model',
      width: 200,
      render: (text) => {
        return <div style={{ wordBreak: 'break-all' }}>{text || '-'}</div>;
      },
    },
    {
      title: t('创建时间'),
      dataIndex: 'create_time',
      width: 180,
      render: (text) => {
        return <div>{formatDateTime(text)}</div>;
      },
    },
    {
      title: t('开始时间'),
      dataIndex: 'start_time',
      width: 180,
      render: (text) => {
        return <div>{formatDateTime(text)}</div>;
      },
    },
    {
      title: t('结束时间'),
      dataIndex: 'end_time',
      width: 180,
      render: (text) => {
        return <div>{formatDateTime(text)}</div>;
      },
    },
    {
      title: t('错误信息'),
      dataIndex: 'error',
      width: 200,
      render: (text) => {
        if (!text) return '-';
        return (
          <div style={{ wordBreak: 'break-all', color: 'red' }}>
            {text}
          </div>
        );
      },
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      width: 100,
      render: (text, record) => {
        return (
          <Space>
            <Button
              theme='light'
              type='tertiary'
              size='small'
              onClick={() => {
                copyText(record.name);
              }}
            >
              {t('复制名称')}
            </Button>
          </Space>
        );
      },
    },
  ];

  const [batchJobs, setBatchJobs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);

  const setBatchJobsFormat = (jobs) => {
    for (let i = 0; i < jobs.length; i++) {
      jobs[i].key = jobs[i].ID;
    }
    setBatchJobs(jobs);
    setTotal(jobs.length);
  };

  const loadBatchJobs = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/batch_job/list');
      const { success, message, data } = res.data;
      if (success) {
        setBatchJobsFormat(data || []);
      } else {
        showError(message || t('加载失败'));
        setBatchJobsFormat([]);
      }
    } catch (error) {
      showError(error.message || t('加载失败'));
      setBatchJobsFormat([]);
    } finally {
      setLoading(false);
    }
  };

  const refresh = async () => {
    await loadBatchJobs();
  };

  const copyText = async (text) => {
    if (await copy(text)) {
      showSuccess(t('已复制到剪贴板！'));
    } else {
      showError(t('无法复制到剪贴板'));
    }
  };

  useEffect(() => {
    loadBatchJobs()
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, []);

  let pageData = batchJobs.slice(
    (activePage - 1) * pageSize,
    activePage * pageSize,
  );

  const handlePageChange = (page) => {
    setActivePage(page);
  };

  return (
    <>
      <div style={{ marginBottom: 16 }}>
        <Button
          theme='light'
          type='primary'
          onClick={refresh}
          loading={loading}
        >
          {t('刷新')}
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={pageData}
        pagination={{
          currentPage: activePage,
          pageSize: pageSize,
          total: total,
          showSizeChanger: true,
          pageSizeOptions: [10, 20, 50, 100],
          formatPageText: (page) =>
            t('第 {{start}} - {{end}} 条，共 {{total}} 条', {
              start: page.currentStart,
              end: page.currentEnd,
              total: total,
            }),
          onPageSizeChange: (size) => {
            setPageSize(size);
            setActivePage(1);
          },
          onPageChange: handlePageChange,
        }}
        loading={loading}
      />
    </>
  );
};

export default BatchJobTable;

